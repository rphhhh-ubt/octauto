package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

func (h Handler) BuyCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	callback := update.CallbackQuery.Message.Message
	langCode := update.CallbackQuery.From.LanguageCode

	tariffs := config.GetTariffs()

	// Если тарифов > 1 → показать меню тарифов
	if len(tariffs) > 1 {
		h.showTariffMenu(ctx, b, callback, langCode, tariffs)
		return
	}

	// Если тарифов = 1 → сразу к ценам с этим тарифом
	if len(tariffs) == 1 {
		h.showTariffPriceMenu(ctx, b, callback, langCode, &tariffs[0])
		return
	}

	// Если тарифов = 0 → старая логика
	h.showLegacyPriceMenu(ctx, b, callback, langCode)
}

// BroadcastBuyCallbackHandler - обработчик кнопки купить из broadcast (всегда новое сообщение)
func (h Handler) BroadcastBuyCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	chatID := update.CallbackQuery.Message.Message.Chat.ID
	langCode := update.CallbackQuery.From.LanguageCode

	tariffs := config.GetTariffs()

	// Если тарифов > 1 → показать меню тарифов
	if len(tariffs) > 1 {
		h.showTariffMenuNew(ctx, b, chatID, langCode, tariffs)
		return
	}

	// Если тарифов = 1 → сразу к ценам с этим тарифом
	if len(tariffs) == 1 {
		h.showTariffPriceMenuNew(ctx, b, chatID, langCode, &tariffs[0])
		return
	}

	// Если тарифов = 0 → старая логика
	h.showLegacyPriceMenuNew(ctx, b, chatID, langCode)
}

// showTariffMenu показывает меню выбора тарифов (редактирует сообщение)
// Requirements: 5.1, 5.2 - показывает кнопку promo tariff если есть активное предложение
func (h Handler) showTariffMenu(ctx context.Context, b *bot.Bot, callback *models.Message, langCode string, tariffs []config.Tariff) {
	keyboard := [][]models.InlineKeyboardButton{}

	// Проверяем наличие активного promo offer у пользователя
	// Property 7: Offer Visibility Based on Expiration
	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.Chat.ID)
	if err == nil && customer != nil && database.HasActivePromoOffer(customer) {
		// Добавляем кнопку promo tariff с эмодзи 🎁 в начало меню
		btnText := h.translation.GetTextTemplate(langCode, "promo_tariff_button", map[string]interface{}{
			"price":  *customer.PromoOfferPrice,
			"months": *customer.PromoOfferMonths,
		})
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: fmt.Sprintf("🎁 %s", btnText), CallbackData: CallbackPromoTariff},
		})
	}

	var tariffButtons []models.InlineKeyboardButton
	for _, tariff := range tariffs {
		tariffButtons = append(tariffButtons, models.InlineKeyboardButton{
			Text:         FormatTariffButtonText(tariff, langCode, h.translation),
			CallbackData: fmt.Sprintf("%s?name=%s", CallbackTariff, tariff.Name),
		})
	}

	// Располагаем кнопки тарифов по одной в ряд для лучшей читаемости
	for _, btn := range tariffButtons {
		keyboard = append(keyboard, []models.InlineKeyboardButton{btn})
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart},
	})

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
		Text: h.translation.GetText(langCode, "select_tariff"),
	})

	if err != nil {
		// Игнорируем ошибки "message is not modified" (двойной клик)
		if strings.Contains(err.Error(), "message is not modified") ||
			strings.Contains(err.Error(), "exactly the same") {
			return
		}
		// Fallback: отправляем новое сообщение если не удалось отредактировать
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    callback.Chat.ID,
			ParseMode: models.ParseModeHTML,
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: keyboard,
			},
			Text: h.translation.GetText(langCode, "select_tariff"),
		})
	}
}

// showTariffMenuNew отправляет новое сообщение с меню тарифов
// Requirements: 5.1, 5.2 - показывает кнопку promo tariff если есть активное предложение
func (h Handler) showTariffMenuNew(ctx context.Context, b *bot.Bot, chatID int64, langCode string, tariffs []config.Tariff) {
	keyboard := [][]models.InlineKeyboardButton{}

	// Проверяем наличие активного promo offer у пользователя
	// Property 7: Offer Visibility Based on Expiration
	customer, err := h.customerRepository.FindByTelegramId(ctx, chatID)
	if err == nil && customer != nil && database.HasActivePromoOffer(customer) {
		// Добавляем кнопку promo tariff с эмодзи 🎁 в начало меню
		btnText := h.translation.GetTextTemplate(langCode, "promo_tariff_button", map[string]interface{}{
			"price":  *customer.PromoOfferPrice,
			"months": *customer.PromoOfferMonths,
		})
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: fmt.Sprintf("🎁 %s", btnText), CallbackData: CallbackPromoTariff},
		})
	}

	var tariffButtons []models.InlineKeyboardButton
	for _, tariff := range tariffs {
		tariffButtons = append(tariffButtons, models.InlineKeyboardButton{
			Text:         FormatTariffButtonText(tariff, langCode, h.translation),
			CallbackData: fmt.Sprintf("%s?name=%s", CallbackTariff, tariff.Name),
		})
	}

	for _, btn := range tariffButtons {
		keyboard = append(keyboard, []models.InlineKeyboardButton{btn})
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart},
	})

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
		Text: h.translation.GetText(langCode, "select_tariff"),
	})

	if err != nil {
		slog.Error("Error sending tariff menu", slog.Any("error", err))
	}
}

// showTariffPriceMenuNew отправляет новое сообщение с ценами тарифа
// Requirements: 5.1, 5.2 - показывает кнопку promo tariff если есть активное предложение
func (h Handler) showTariffPriceMenuNew(ctx context.Context, b *bot.Bot, chatID int64, langCode string, tariff *config.Tariff) {
	keyboard := [][]models.InlineKeyboardButton{}

	// Проверяем наличие активного promo offer у пользователя
	// Property 7: Offer Visibility Based on Expiration
	customer, err := h.customerRepository.FindByTelegramId(ctx, chatID)
	if err == nil && customer != nil && database.HasActivePromoOffer(customer) {
		// Добавляем кнопку promo tariff с эмодзи 🎁 в начало меню
		btnText := h.translation.GetTextTemplate(langCode, "promo_tariff_button", map[string]interface{}{
			"price":  *customer.PromoOfferPrice,
			"months": *customer.PromoOfferMonths,
		})
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: fmt.Sprintf("🎁 %s", btnText), CallbackData: CallbackPromoTariff},
		})
	}

	var priceButtons []models.InlineKeyboardButton

	if tariff.Price1 > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_1", map[string]interface{}{"price": tariff.Price1}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d&tariff=%s", CallbackSell, 1, tariff.Price1, tariff.Name),
		})
	}

	if tariff.Price3 > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_3", map[string]interface{}{"price": tariff.Price3}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d&tariff=%s", CallbackSell, 3, tariff.Price3, tariff.Name),
		})
	}

	if tariff.Price6 > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_6", map[string]interface{}{"price": tariff.Price6}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d&tariff=%s", CallbackSell, 6, tariff.Price6, tariff.Name),
		})
	}

	if tariff.Price12 > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_12", map[string]interface{}{"price": tariff.Price12}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d&tariff=%s", CallbackSell, 12, tariff.Price12, tariff.Name),
		})
	}

	if len(priceButtons) == 4 {
		keyboard = append(keyboard, priceButtons[:2])
		keyboard = append(keyboard, priceButtons[2:])
	} else if len(priceButtons) > 0 {
		keyboard = append(keyboard, priceButtons)
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart},
	})

	pricingText := h.translation.GetTextTemplate(langCode, "pricing_info", map[string]interface{}{
		"devices": tariff.Devices,
	})

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
		Text: pricingText,
	})

	if err != nil {
		slog.Error("Error sending tariff price menu", slog.Any("error", err))
	}
}

// showTariffPriceMenu показывает меню цен для конкретного тарифа
// Requirements: 5.1, 5.2 - показывает кнопку promo tariff если есть активное предложение
func (h Handler) showTariffPriceMenu(ctx context.Context, b *bot.Bot, callback *models.Message, langCode string, tariff *config.Tariff) {
	keyboard := [][]models.InlineKeyboardButton{}

	// Проверяем наличие активного promo offer у пользователя
	// Property 7: Offer Visibility Based on Expiration
	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.Chat.ID)
	if err == nil && customer != nil && database.HasActivePromoOffer(customer) {
		// Добавляем кнопку promo tariff с эмодзи 🎁 в начало меню
		btnText := h.translation.GetTextTemplate(langCode, "promo_tariff_button", map[string]interface{}{
			"price":  *customer.PromoOfferPrice,
			"months": *customer.PromoOfferMonths,
		})
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: fmt.Sprintf("🎁 %s", btnText), CallbackData: CallbackPromoTariff},
		})
	}

	var priceButtons []models.InlineKeyboardButton

	if tariff.Price1 > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_1", map[string]interface{}{"price": tariff.Price1}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d&tariff=%s", CallbackSell, 1, tariff.Price1, tariff.Name),
		})
	}

	if tariff.Price3 > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_3", map[string]interface{}{"price": tariff.Price3}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d&tariff=%s", CallbackSell, 3, tariff.Price3, tariff.Name),
		})
	}

	if tariff.Price6 > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_6", map[string]interface{}{"price": tariff.Price6}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d&tariff=%s", CallbackSell, 6, tariff.Price6, tariff.Name),
		})
	}

	if tariff.Price12 > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_12", map[string]interface{}{"price": tariff.Price12}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d&tariff=%s", CallbackSell, 12, tariff.Price12, tariff.Name),
		})
	}

	if len(priceButtons) == 4 {
		keyboard = append(keyboard, priceButtons[:2])
		keyboard = append(keyboard, priceButtons[2:])
	} else if len(priceButtons) > 0 {
		keyboard = append(keyboard, priceButtons)
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart},
	})

	pricingText := h.translation.GetTextTemplate(langCode, "pricing_info", map[string]interface{}{
		"devices": tariff.Devices,
	})

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
		Text: pricingText,
	})

	if err != nil {
		// Игнорируем ошибки "message is not modified" (двойной клик)
		if strings.Contains(err.Error(), "message is not modified") ||
			strings.Contains(err.Error(), "exactly the same") {
			return
		}
		// Fallback: отправляем новое сообщение если не удалось отредактировать
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    callback.Chat.ID,
			ParseMode: models.ParseModeHTML,
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: keyboard,
			},
			Text: pricingText,
		})
	}
}

// showLegacyPriceMenu показывает старое меню цен (без тарифов)
// Requirements: 5.1, 5.2 - показывает кнопку promo tariff если есть активное предложение
func (h Handler) showLegacyPriceMenu(ctx context.Context, b *bot.Bot, callback *models.Message, langCode string) {
	keyboard := [][]models.InlineKeyboardButton{}

	// Проверяем наличие активного promo offer у пользователя
	// Property 7: Offer Visibility Based on Expiration
	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.Chat.ID)
	if err == nil && customer != nil && database.HasActivePromoOffer(customer) {
		// Добавляем кнопку promo tariff с эмодзи 🎁 в начало меню
		btnText := h.translation.GetTextTemplate(langCode, "promo_tariff_button", map[string]interface{}{
			"price":  *customer.PromoOfferPrice,
			"months": *customer.PromoOfferMonths,
		})
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: fmt.Sprintf("🎁 %s", btnText), CallbackData: CallbackPromoTariff},
		})
	}

	var priceButtons []models.InlineKeyboardButton

	if config.Price1() > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_1", map[string]interface{}{"price": config.Price1()}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d", CallbackSell, 1, config.Price1()),
		})
	}

	if config.Price3() > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_3", map[string]interface{}{"price": config.Price3()}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d", CallbackSell, 3, config.Price3()),
		})
	}

	if config.Price6() > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_6", map[string]interface{}{"price": config.Price6()}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d", CallbackSell, 6, config.Price6()),
		})
	}

	if config.Price12() > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_12", map[string]interface{}{"price": config.Price12()}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d", CallbackSell, 12, config.Price12()),
		})
	}

	if len(priceButtons) == 4 {
		keyboard = append(keyboard, priceButtons[:2])
		keyboard = append(keyboard, priceButtons[2:])
	} else if len(priceButtons) > 0 {
		keyboard = append(keyboard, priceButtons)
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart},
	})

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
		Text: h.translation.GetText(langCode, "pricing_info_legacy"),
	})

	if err != nil {
		// Игнорируем ошибки "message is not modified" (двойной клик)
		if strings.Contains(err.Error(), "message is not modified") ||
			strings.Contains(err.Error(), "exactly the same") {
			return
		}
		// Fallback: отправляем новое сообщение если не удалось отредактировать
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    callback.Chat.ID,
			ParseMode: models.ParseModeHTML,
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: keyboard,
			},
			Text: h.translation.GetText(langCode, "pricing_info_legacy"),
		})
	}
}

func (h Handler) SellCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	callback := update.CallbackQuery.Message.Message
	callbackQuery := parseCallbackData(update.CallbackQuery.Data)
	langCode := update.CallbackQuery.From.LanguageCode
	month := callbackQuery["month"]
	amount := callbackQuery["amount"]
	tariff := callbackQuery["tariff"] // Получаем имя тарифа из callback

	// Проверяем есть ли у пользователя сохранённый метод оплаты — если да, включаем recurring по умолчанию
	recurringEnabled := false
	if config.IsRecurringPaymentsEnabled() {
		customer, err := h.customerRepository.FindByTelegramId(ctx, callback.Chat.ID)
		if err == nil && customer != nil && customer.PaymentMethodID != nil {
			recurringEnabled = true
		}
	}

	h.showPaymentMethodsWithRecurring(ctx, b, callback, langCode, month, amount, tariff, recurringEnabled)
}

func (h Handler) PaymentCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	callback := update.CallbackQuery.Message.Message
	callbackQuery := parseCallbackData(update.CallbackQuery.Data)
	
	// Поддержка коротких и длинных ключей для обратной совместимости
	monthStr := callbackQuery["m"]
	if monthStr == "" {
		monthStr = callbackQuery["month"]
	}
	month, err := strconv.Atoi(monthStr)
	if err != nil {
		slog.Error("Error getting month from query", "error", err)
		return
	}

	invoiceTypeStr := callbackQuery["t"]
	if invoiceTypeStr == "" {
		invoiceTypeStr = callbackQuery["invoiceType"]
	}
	invoiceType := database.InvoiceType(invoiceTypeStr)
	
	tariffName := callbackQuery["n"]
	if tariffName == "" {
		tariffName = callbackQuery["tariff"]
	}
	
	isWinback := callbackQuery["winback"] == "true" || callbackQuery["w"] == "1"
	isRecurring := callbackQuery["recurring"] == "true" || callbackQuery["r"] == "1"
	isPromoTariff := callbackQuery["pt"] == "1"

	// Получаем customer сразу — нужен для winback, promo tariff и далее
	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.Chat.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}
	if customer == nil {
		slog.Error("customer not exist", "chatID", callback.Chat.ID)
		return
	}

	// Определяем цену и месяцы
	var price int
	if isPromoTariff {
		// Property 8: Purchase Uses Offer Parameters
		// Для promo tariff берём параметры из сохранённого предложения в БД
		if customer.PromoOfferPrice == nil || customer.PromoOfferMonths == nil {
			slog.Error("Cannot get promo tariff parameters - offer not found", "customerId", customer.ID)
			return
		}
		// Проверяем что предложение не истекло
		if !database.HasActivePromoOffer(customer) {
			slog.Warn("Promo tariff offer expired", "customerId", customer.ID)
			return
		}
		price = *customer.PromoOfferPrice
		month = *customer.PromoOfferMonths // Переопределяем месяцы из предложения
		slog.Debug("Using promo tariff price from saved offer", "price", price, "months", month)
	} else if isWinback {
		// Для winback берём цену из сохранённого предложения в БД
		// Это гарантирует что пользователь заплатит ту цену, которую видел в уведомлении
		if customer.WinbackOfferPrice == nil {
			slog.Error("Cannot get winback price - offer not found", "customerId", customer.ID)
			return
		}
		price = *customer.WinbackOfferPrice
		slog.Debug("Using winback price from saved offer", "price", price)
	} else if tariffName != "" {
		tariff := config.GetTariffByName(tariffName)
		if tariff != nil {
			if invoiceType == database.InvoiceTypeTelegram {
				price = tariff.StarsPrice(month)
			} else {
				price = tariff.Price(month)
			}
			slog.Debug("Using tariff price from config", "tariff", tariffName, "price", price, "invoiceType", invoiceType)
		} else {
			slog.Warn("Tariff not found, using default price", "tariff", tariffName)
			if invoiceType == database.InvoiceTypeTelegram {
				price = config.StarsPrice(month)
			} else {
				price = config.Price(month)
			}
		}
	} else {
		// Legacy flow без тарифов — используем глобальные цены
		if invoiceType == database.InvoiceTypeTelegram {
			price = config.StarsPrice(month)
		} else {
			price = config.Price(month)
		}
	}

	ctxWithUsername := context.WithValue(ctx, "username", update.CallbackQuery.From.Username)

	// Передаём tariffName в CreatePurchase (nil если пустой)
	var tariffNamePtr *string
	if tariffName != "" {
		tariffNamePtr = &tariffName
	}

	// Определяем deviceLimit из сохранённого предложения в БД
	// Property 8: Purchase Uses Offer Parameters - для promo tariff используем promo_offer_devices
	var deviceLimit *int
	if isPromoTariff && customer.PromoOfferDevices != nil {
		deviceLimit = customer.PromoOfferDevices
		slog.Info("Creating promo tariff purchase", "price", price, "months", month, "devices", *deviceLimit)
	} else if isWinback && customer.WinbackOfferDevices != nil {
		// Для winback берём deviceLimit из сохранённого предложения в БД
		// Это гарантирует консистентность с тем что пользователь видел в уведомлении
		deviceLimit = customer.WinbackOfferDevices
		slog.Info("Creating winback purchase", "price", price, "months", month, "devices", *deviceLimit)
	}

	// Определяем нужно ли сохранять способ оплаты для автопродления
	// Автопродление поддерживается только для YooKassa и если функция включена
	savePaymentMethod := isRecurring && invoiceType == database.InvoiceTypeYookasa && config.IsRecurringPaymentsEnabled()

	if savePaymentMethod {
		slog.Info("Creating payment with recurring enabled", "price", price, "months", month, "tariff", tariffName)
	}

	paymentURL, purchaseId, err := h.paymentService.CreatePurchaseWithRecurring(ctxWithUsername, float64(price), month, customer, invoiceType, tariffNamePtr, deviceLimit, savePaymentMethod)
	if err != nil {
		slog.Error("Error creating payment", "error", err)
		return
	}

	langCode := update.CallbackQuery.From.LanguageCode

	// Формируем callback для кнопки "назад" с учётом тарифа, winback и promo tariff
	var backCallback string
	if isPromoTariff {
		backCallback = CallbackPromoTariff // Для promo tariff возвращаемся к выбору оплаты
	} else if isWinback {
		backCallback = CallbackStart // Для winback возвращаемся в главное меню
	} else if tariffName != "" {
		backCallback = fmt.Sprintf("%s?month=%d&amount=%d&tariff=%s", CallbackSell, month, price, tariffName)
	} else {
		backCallback = fmt.Sprintf("%s?month=%d&amount=%d", CallbackSell, month, price)
	}

	var keyboard [][]models.InlineKeyboardButton

	// Кнопки Оплатить и Назад
	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "pay_button"), URL: paymentURL},
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: backCallback},
	})

	// Показываем чекбокс автопродления только для YooKassa
	// Для winback показываем только если WINBACK_RECURRING_ENABLED=true
	// Для promo tariff не показываем чекбокс автопродления
	showRecurringCheckbox := invoiceType == database.InvoiceTypeYookasa && config.IsRecurringPaymentsEnabled() && !isPromoTariff && (!isWinback || config.IsWinbackRecurringEnabled())
	if showRecurringCheckbox {
		checkboxText := "☐ " + h.translation.GetText(langCode, "recurring_checkbox")
		if isRecurring {
			checkboxText = "☑ " + h.translation.GetText(langCode, "recurring_checkbox")
		}
		// Формируем callback для toggle с текущими параметрами
		toggleCallback := fmt.Sprintf("%s?m=%d&a=%d&t=%s", CallbackRecurringToggle, month, price, invoiceType)
		if tariffName != "" {
			toggleCallback += fmt.Sprintf("&n=%s", tariffName)
		}
		if isRecurring {
			toggleCallback += "&r=1"
		}
		if isWinback {
			toggleCallback += "&w=1"
		}
		if isPromoTariff {
			toggleCallback += "&pt=1"
		}
		toggleCallback = SafeCallbackData(toggleCallback)
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: checkboxText, CallbackData: toggleCallback},
		})
	}

	message, err := b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
	})
	if err != nil {
		slog.Error("Error updating sell message", "error", err)
		return
	}
	h.cache.Set(purchaseId, message.ID)
}

func (h Handler) PreCheckoutCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, err := b.AnswerPreCheckoutQuery(ctx, &bot.AnswerPreCheckoutQueryParams{
		PreCheckoutQueryID: update.PreCheckoutQuery.ID,
		OK:                 true,
	})
	if err != nil {
		slog.Error("Error sending answer pre checkout query", "error", err)
	}
}

func (h Handler) SuccessPaymentHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	payload := strings.Split(update.Message.SuccessfulPayment.InvoicePayload, "&")
	purchaseId, err := strconv.Atoi(payload[0])
	username := payload[1]
	if err != nil {
		slog.Error("Error parsing purchase id", "error", err)
		return
	}

	ctxWithUsername := context.WithValue(ctx, "username", username)
	err = h.paymentService.ProcessPurchaseById(ctxWithUsername, int64(purchaseId))
	if err != nil {
		slog.Error("Error processing purchase", "error", err)
	}
}

func parseCallbackData(data string) map[string]string {
	result := make(map[string]string)

	parts := strings.Split(data, "?")
	if len(parts) < 2 {
		return result
	}

	params := strings.Split(parts[1], "&")
	for _, param := range params {
		kv := strings.SplitN(param, "=", 2)
		if len(kv) == 2 {
			result[kv[0]] = kv[1]
		}
	}

	return result
}

// RecurringToggleCallbackHandler обрабатывает переключение чекбокса автопродления
// Переключает состояние recurring и перенаправляет на PaymentCallbackHandler с новым состоянием
func (h Handler) RecurringToggleCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callbackQuery := parseCallbackData(update.CallbackQuery.Data)
	currentRecurring := callbackQuery["recurring"] == "true" || callbackQuery["r"] == "1"
	newRecurring := !currentRecurring

	// Поддержка коротких и длинных ключей
	month := callbackQuery["m"]
	if month == "" {
		month = callbackQuery["month"]
	}
	amount := callbackQuery["a"]
	if amount == "" {
		amount = callbackQuery["amount"]
	}
	tariff := callbackQuery["n"]
	if tariff == "" {
		tariff = callbackQuery["tariff"]
	}
	invoiceType := callbackQuery["t"]
	if invoiceType == "" {
		invoiceType = callbackQuery["invoiceType"]
	}
	isWinback := callbackQuery["winback"] == "true" || callbackQuery["w"] == "1"
	isPromoTariff := callbackQuery["pt"] == "1"

	// Формируем новый callback data с переключённым состоянием recurring
	newCallbackData := fmt.Sprintf("%s?m=%s&t=%s&a=%s", CallbackPayment, month, invoiceType, amount)
	if tariff != "" {
		newCallbackData += fmt.Sprintf("&n=%s", tariff)
	}
	if newRecurring {
		newCallbackData += "&r=1"
	}
	if isWinback {
		newCallbackData += "&w=1"
	}
	if isPromoTariff {
		newCallbackData += "&pt=1"
	}

	// Подменяем callback data и вызываем PaymentCallbackHandler
	update.CallbackQuery.Data = newCallbackData
	h.PaymentCallbackHandler(ctx, b, update)
}

// showPaymentMethodsWithRecurring показывает меню выбора способа оплаты с чекбоксом автопродления
func (h Handler) showPaymentMethodsWithRecurring(ctx context.Context, b *bot.Bot, callback *models.Message, langCode string, month string, amount string, tariff string, recurringEnabled bool) {
	// Формируем базовый callback с тарифом и recurring (короткие ключи для лимита 64 байта)
	buildPaymentCallback := func(invoiceType database.InvoiceType) string {
		base := fmt.Sprintf("%s?m=%s&t=%s&a=%s", CallbackPayment, month, invoiceType, amount)
		if tariff != "" {
			base += fmt.Sprintf("&n=%s", tariff)
		}
		if recurringEnabled {
			base += "&r=1"
		}
		return SafeCallbackData(base)
	}

	var keyboard [][]models.InlineKeyboardButton

	// Сохранённый способ оплаты показываем ПЕРВЫМ (сверху) если есть
	if config.IsYookasaEnabled() && config.IsRecurringPaymentsEnabled() {
		customer, err := h.customerRepository.FindByTelegramId(ctx, callback.Chat.ID)
		if err == nil && customer != nil && customer.PaymentMethodID != nil {
			// Передаём параметры чтобы кнопка "Назад" вернула в это меню
			savedCallback := fmt.Sprintf("%s?m=%s&a=%s", CallbackSavedPaymentMethods, month, amount)
			if tariff != "" {
				savedCallback += fmt.Sprintf("&n=%s", tariff)
			}
			keyboard = append(keyboard, []models.InlineKeyboardButton{
				{Text: h.translation.GetText(langCode, "saved_payment_methods_button"), CallbackData: savedCallback},
			})
		}
	}

	if config.IsCryptoPayEnabled() {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "crypto_button"), CallbackData: buildPaymentCallback(database.InvoiceTypeCrypto)},
		})
	}

	if config.IsYookasaEnabled() {
		// Кнопка оплаты картой
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "card_button"), CallbackData: buildPaymentCallback(database.InvoiceTypeYookasa)},
		})
	}

	if config.IsTelegramStarsEnabled() {
		shouldShowStarsButton := true

		if config.RequirePaidPurchaseForStars() {
			customer, err := h.customerRepository.FindByTelegramId(ctx, callback.Chat.ID)
			if err != nil {
				slog.Error("Error finding customer for stars check", "error", err)
				shouldShowStarsButton = false
			} else if customer != nil {
				paidPurchase, err := h.purchaseRepository.FindSuccessfulPaidPurchaseByCustomer(ctx, customer.ID)
				if err != nil {
					slog.Error("Error checking paid purchase", "error", err)
					shouldShowStarsButton = false
				} else if paidPurchase == nil {
					shouldShowStarsButton = false
				}
			} else {
				shouldShowStarsButton = false
			}
		}

		if shouldShowStarsButton {
			keyboard = append(keyboard, []models.InlineKeyboardButton{
				{Text: h.translation.GetText(langCode, "stars_button"), CallbackData: buildPaymentCallback(database.InvoiceTypeTelegram)},
			})
		}
	}

	if config.GetTributeWebHookUrl() != "" {
		// Если указан тариф — используем его tribute URL, иначе общий
		tributeURL := config.GetTributePaymentUrl()
		if tariff != "" {
			t := config.GetTariffByName(tariff)
			if t != nil && t.TributeURL != "" {
				tributeURL = t.TributeURL
			}
		}
		if tributeURL != "" {
			keyboard = append(keyboard, []models.InlineKeyboardButton{
				{Text: h.translation.GetText(langCode, "tribute_button"), URL: tributeURL},
			})
		}
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackBuy},
	})

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		Text:      h.translation.GetText(langCode, "select_payment"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
	})

	if err != nil {
		slog.Error("Error updating payment methods menu", "error", err)
	}
}

// RecurringDisableCallbackHandler обрабатывает отключение автопродления
// Requirements: 3.1, 3.2
func (h Handler) RecurringDisableCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	callback := update.CallbackQuery.Message.Message
	langCode := update.CallbackQuery.From.LanguageCode
	telegramID := update.CallbackQuery.From.ID

	// Находим пользователя
	customer, err := h.customerRepository.FindByTelegramId(ctx, telegramID)
	if err != nil {
		slog.Error("Error finding customer for recurring disable", "error", err)
		return
	}
	if customer == nil {
		slog.Error("Customer not found for recurring disable", "telegramID", telegramID)
		return
	}

	// Отключаем автопродление и очищаем payment_method_id
	err = h.customerRepository.DisableRecurring(ctx, customer.ID)
	if err != nil {
		slog.Error("Error disabling recurring", "customerID", customer.ID, "error", err)
		return
	}

	slog.Info("Recurring disabled by user", "customerID", customer.ID, "telegramID", telegramID)

	// Отправляем подтверждение
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		ParseMode: models.ParseModeHTML,
		Text:      h.translation.GetText(langCode, "recurring_disabled_confirmation"),
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_to_menu"), CallbackData: CallbackStart}},
			},
		},
	})
	if err != nil {
		// Если не удалось отредактировать, отправляем новое сообщение
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    callback.Chat.ID,
			ParseMode: models.ParseModeHTML,
			Text:      h.translation.GetText(langCode, "recurring_disabled_confirmation"),
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{
					{{Text: h.translation.GetText(langCode, "back_to_menu"), CallbackData: CallbackStart}},
				},
			},
		})
	}
}

// DeletePaymentMethodCallbackHandler удаляет сохранённый способ оплаты
func (h Handler) DeletePaymentMethodCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	callback := update.CallbackQuery.Message.Message
	langCode := update.CallbackQuery.From.LanguageCode
	telegramID := update.CallbackQuery.From.ID

	customer, err := h.customerRepository.FindByTelegramId(ctx, telegramID)
	if err != nil {
		slog.Error("Error finding customer for delete payment method", "error", err)
		return
	}
	if customer == nil {
		slog.Error("Customer not found for delete payment method", "telegramID", telegramID)
		return
	}

	// Удаляем способ оплаты и отключаем автопродление
	err = h.customerRepository.DeletePaymentMethod(ctx, customer.ID)
	if err != nil {
		slog.Error("Error deleting payment method", "customerID", customer.ID, "error", err)
		return
	}

	slog.Info("Payment method deleted by user", "customerID", customer.ID, "telegramID", telegramID)

	// Отправляем подтверждение
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		ParseMode: models.ParseModeHTML,
		Text:      h.translation.GetText(langCode, "payment_method_deleted"),
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_to_menu"), CallbackData: CallbackStart}},
			},
		},
	})
	if err != nil {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    callback.Chat.ID,
			ParseMode: models.ParseModeHTML,
			Text:      h.translation.GetText(langCode, "payment_method_deleted"),
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{
					{{Text: h.translation.GetText(langCode, "back_to_menu"), CallbackData: CallbackStart}},
				},
			},
		})
	}
}

// showLegacyPriceMenuNew показывает старое меню цен (новое сообщение)
// Requirements: 5.1, 5.2 - показывает кнопку promo tariff если есть активное предложение
func (h Handler) showLegacyPriceMenuNew(ctx context.Context, b *bot.Bot, chatID int64, langCode string) {
	keyboard := [][]models.InlineKeyboardButton{}

	// Проверяем наличие активного promo offer у пользователя
	// Property 7: Offer Visibility Based on Expiration
	customer, err := h.customerRepository.FindByTelegramId(ctx, chatID)
	if err == nil && customer != nil && database.HasActivePromoOffer(customer) {
		// Добавляем кнопку promo tariff с эмодзи 🎁 в начало меню
		btnText := h.translation.GetTextTemplate(langCode, "promo_tariff_button", map[string]interface{}{
			"price":  *customer.PromoOfferPrice,
			"months": *customer.PromoOfferMonths,
		})
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: fmt.Sprintf("🎁 %s", btnText), CallbackData: CallbackPromoTariff},
		})
	}

	var priceButtons []models.InlineKeyboardButton

	if config.Price1() > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_1", map[string]interface{}{"price": config.Price1()}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d", CallbackSell, 1, config.Price1()),
		})
	}

	if config.Price3() > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_3", map[string]interface{}{"price": config.Price3()}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d", CallbackSell, 3, config.Price3()),
		})
	}

	if config.Price6() > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_6", map[string]interface{}{"price": config.Price6()}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d", CallbackSell, 6, config.Price6()),
		})
	}

	if config.Price12() > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_12", map[string]interface{}{"price": config.Price12()}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d", CallbackSell, 12, config.Price12()),
		})
	}

	if len(priceButtons) == 4 {
		keyboard = append(keyboard, priceButtons[:2])
		keyboard = append(keyboard, priceButtons[2:])
	} else if len(priceButtons) > 0 {
		keyboard = append(keyboard, priceButtons)
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart},
	})

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
		Text: h.translation.GetText(langCode, "pricing_info_legacy"),
	})

	if err != nil {
		slog.Error("Error sending buy message", slog.Any("error", err))
	}
}

// SavedPaymentMethodsCallbackHandler показывает сохранённые способы оплаты
// Requirements: 4.1, 4.2
func (h Handler) SavedPaymentMethodsCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	callback := update.CallbackQuery.Message.Message
	langCode := update.CallbackQuery.From.LanguageCode
	telegramID := update.CallbackQuery.From.ID

	// Парсим callback data для определения источника вызова
	callbackQuery := parseCallbackData(update.CallbackQuery.Data)
	fromNotification := callbackQuery["from"] == "notification"

	// Находим пользователя
	customer, err := h.customerRepository.FindByTelegramId(ctx, telegramID)
	if err != nil {
		slog.Error("Error finding customer for saved payment methods", "error", err)
		return
	}
	if customer == nil {
		slog.Error("Customer not found for saved payment methods", "telegramID", telegramID)
		return
	}

	var text string
	var keyboard [][]models.InlineKeyboardButton

	// Если нет сохранённого способа оплаты
	if customer.PaymentMethodID == nil {
		text = h.translation.GetText(langCode, "saved_payment_methods_empty")
		if fromNotification {
			keyboard = [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "close_button"), CallbackData: CallbackCloseMessage}},
			}
		} else {
			// Формируем callback для возврата в меню способов оплаты
			backCallback := CallbackBuy
			month := callbackQuery["m"]
			amount := callbackQuery["a"]
			tariff := callbackQuery["n"]
			if month != "" && amount != "" {
				backCallback = fmt.Sprintf("%s?month=%s&amount=%s", CallbackSell, month, amount)
				if tariff != "" {
					backCallback += fmt.Sprintf("&tariff=%s", tariff)
				}
			}
			keyboard = [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: backCallback}},
			}
		}
	} else {
		// Есть сохранённый способ оплаты
		text = h.translation.GetText(langCode, "saved_payment_methods_title")

		if customer.RecurringEnabled {
			// Автопродление включено - показываем детали
			tariffName := "—"
			if customer.RecurringTariffName != nil {
				tariffName = *customer.RecurringTariffName
			}

			amount := 0
			if customer.RecurringAmount != nil {
				amount = *customer.RecurringAmount
			}

			nextCharge := "—"
			if customer.ExpireAt != nil {
				nextCharge = customer.ExpireAt.Format("02.01.2006")
			}

			text += h.translation.GetTextTemplate(langCode, "saved_payment_methods_status_enabled", map[string]interface{}{
				"tariff":      tariffName,
				"amount":      amount,
				"next_charge": nextCharge,
			})
		} else {
			// Автопродление отключено, но карта сохранена
			text += h.translation.GetText(langCode, "saved_payment_methods_status_disabled")
		}

		keyboard = [][]models.InlineKeyboardButton{
			{{Text: h.translation.GetText(langCode, "delete_saved_payment_method"), CallbackData: CallbackDeletePaymentMethod}},
		}
		if fromNotification {
			keyboard = append(keyboard, []models.InlineKeyboardButton{
				{Text: h.translation.GetText(langCode, "close_button"), CallbackData: CallbackCloseMessage},
			})
		} else {
			// Формируем callback для возврата в меню способов оплаты
			backCallback := CallbackBuy
			month := callbackQuery["m"]
			amount := callbackQuery["a"]
			tariff := callbackQuery["n"]
			if month != "" && amount != "" {
				backCallback = fmt.Sprintf("%s?month=%s&amount=%s", CallbackSell, month, amount)
				if tariff != "" {
					backCallback += fmt.Sprintf("&tariff=%s", tariff)
				}
			}
			keyboard = append(keyboard, []models.InlineKeyboardButton{
				{Text: h.translation.GetText(langCode, "back_button"), CallbackData: backCallback},
			})
		}
	}

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		ParseMode: models.ParseModeHTML,
		Text:      text,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
	})
	if err != nil {
		// Если не удалось отредактировать, отправляем новое сообщение с кнопкой закрытия
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    callback.Chat.ID,
			ParseMode: models.ParseModeHTML,
			Text:      text,
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: h.savedPaymentMethodsKeyboardWithClose(langCode, customer),
			},
		})
	}
}

// savedPaymentMethodsKeyboardWithClose формирует клавиатуру для нового сообщения с кнопкой закрытия
func (h Handler) savedPaymentMethodsKeyboardWithClose(langCode string, customer *database.Customer) [][]models.InlineKeyboardButton {
	var keyboard [][]models.InlineKeyboardButton

	if customer.PaymentMethodID != nil {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "delete_saved_payment_method"), CallbackData: CallbackDeletePaymentMethod},
		})
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "close_button"), CallbackData: CallbackCloseMessage},
	})

	return keyboard
}

// CloseMessageCallbackHandler удаляет сообщение при нажатии на кнопку "Закрыть"
func (h Handler) CloseMessageCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	callback := update.CallbackQuery.Message.Message
	_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
	})
}
