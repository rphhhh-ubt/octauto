package handler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/utils"
)

// WinbackCallbackHandler обрабатывает активацию winback предложения
// Показывает кнопки оплаты с ценой из winback предложения
// Requirements: 3.4, 3.5
func (h Handler) WinbackCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	callback := update.CallbackQuery.Message.Message
	langCode := update.CallbackQuery.From.LanguageCode
	telegramID := update.CallbackQuery.From.ID

	// Получаем customer
	customer, err := h.customerRepository.FindByTelegramId(ctx, telegramID)
	if err != nil {
		slog.Error("Error finding customer for winback", "error", err, "telegramId", utils.MaskHalfInt64(telegramID))
		return
	}
	if customer == nil {
		slog.Error("Customer not found for winback", "telegramId", utils.MaskHalfInt64(telegramID))
		return
	}

	// Проверяем наличие winback предложения
	if customer.WinbackOfferSentAt == nil {
		slog.Warn("No winback offer for customer", "customerId", utils.MaskHalfInt64(customer.ID))
		h.sendWinbackError(ctx, b, callback, langCode, "winback_no_offer")
		return
	}

	// Проверяем срок действия предложения (Property 4: Winback Offer Activation Validity)
	// Предложение действительно только когда WinbackOfferExpiresAt > current time
	if !IsWinbackOfferValid(customer.WinbackOfferExpiresAt, time.Now()) {
		slog.Info("Winback offer expired", "customerId", utils.MaskHalfInt64(customer.ID),
			"expiresAt", customer.WinbackOfferExpiresAt)
		h.sendWinbackExpired(ctx, b, callback, langCode)
		return
	}

	// Получаем параметры предложения
	price := customer.WinbackOfferPrice
	months := customer.WinbackOfferMonths

	if price == nil || months == nil {
		slog.Error("Winback offer has nil parameters", "customerId", utils.MaskHalfInt64(customer.ID))
		h.sendWinbackError(ctx, b, callback, langCode, "winback_error")
		return
	}

	slog.Info("Showing winback payment options",
		"customerId", utils.MaskHalfInt64(customer.ID),
		"price", *price,
		"months", *months)

	// Показываем кнопки оплаты (как в SellCallbackHandler)
	h.showWinbackPaymentOptions(ctx, b, callback, langCode, *price, *months)
}

// IsWinbackOfferValid проверяет действительность winback предложения
// Property 4: Winback Offer Activation Validity
// Предложение действительно только когда expiresAt > currentTime
func IsWinbackOfferValid(expiresAt *time.Time, currentTime time.Time) bool {
	if expiresAt == nil {
		return false
	}
	return expiresAt.After(currentTime)
}

// WinbackPurchaseParams содержит параметры для создания winback покупки
// Property 6: Winback Purchase Uses Offer Device Limit
type WinbackPurchaseParams struct {
	Price       int  // цена в рублях
	Devices     int  // hwidDeviceLimit из WinbackOfferDevices
	Months      int  // период подписки
	Days        int  // период в днях
	IsValid     bool // валидны ли параметры
}

// ExtractWinbackPurchaseParams извлекает параметры покупки из winback предложения
// Property 6: Winback Purchase Uses Offer Device Limit
// hwidDeviceLimit устанавливается из WinbackOfferDevices
func ExtractWinbackPurchaseParams(
	offerPrice *int,
	offerDevices *int,
	offerMonths *int,
	daysInMonth int,
) WinbackPurchaseParams {
	// Если любой параметр nil - предложение невалидно
	if offerPrice == nil || offerDevices == nil || offerMonths == nil {
		return WinbackPurchaseParams{IsValid: false}
	}

	return WinbackPurchaseParams{
		Price:   *offerPrice,
		Devices: *offerDevices, // hwidDeviceLimit берётся напрямую из WinbackOfferDevices
		Months:  *offerMonths,
		Days:    *offerMonths * daysInMonth,
		IsValid: true,
	}
}

// showWinbackPaymentOptions показывает кнопки оплаты для winback предложения
// Аналогично SellCallbackHandler, но с параметрами из winback
func (h Handler) showWinbackPaymentOptions(ctx context.Context, b *bot.Bot, callback *models.Message, langCode string, price int, months int) {
	// Формируем callback для оплаты с пометкой winback (короткие ключи для лимита 64 байта)
	buildPaymentCallback := func(invoiceType database.InvoiceType) string {
		return fmt.Sprintf("%s?m=%d&t=%s&a=%d&w=1", CallbackPayment, months, invoiceType, price)
	}

	var keyboard [][]models.InlineKeyboardButton

	if config.IsCryptoPayEnabled() {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "crypto_button"), CallbackData: buildPaymentCallback(database.InvoiceTypeCrypto)},
		})
	}

	if config.IsYookasaEnabled() {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "card_button"), CallbackData: buildPaymentCallback(database.InvoiceTypeYookasa)},
		})
	}

	if config.IsTelegramStarsEnabled() {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "stars_button"), CallbackData: buildPaymentCallback(database.InvoiceTypeTelegram)},
		})
	}

	if config.GetTributeWebHookUrl() != "" {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "tribute_button"), URL: config.GetTributePaymentUrl()},
		})
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart},
	})

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		Text:      h.translation.GetText(langCode, "winback_select_payment"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
	})

	if err != nil {
		slog.Error("Error showing winback payment options", "error", err)
	}
}

// sendWinbackExpired отправляет сообщение об истечении срока предложения
func (h Handler) sendWinbackExpired(ctx context.Context, b *bot.Bot, callback *models.Message, langCode string) {
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		Text:      h.translation.GetText(langCode, "winback_expired"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "buy_button"), CallbackData: CallbackBuy}},
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart}},
			},
		},
	})
	if err != nil {
		slog.Error("Error sending winback expired message", "error", err)
	}
}

// sendWinbackError отправляет сообщение об ошибке
func (h Handler) sendWinbackError(ctx context.Context, b *bot.Bot, callback *models.Message, langCode string, errorKey string) {
	text := h.translation.GetText(langCode, errorKey)
	if text == "" {
		text = h.translation.GetText(langCode, "winback_error")
	}
	
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart}},
			},
		},
	})
	if err != nil {
		slog.Error("Error sending winback error message", "error", err)
	}
}
