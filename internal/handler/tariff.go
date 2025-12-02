package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/translation"
)

// FormatTariffButtonText форматирует текст кнопки тарифа с учётом локализации
// Формат: "{emoji} {Name} — до {Devices} устройств — от N ₽/мес"
func FormatTariffButtonText(tariff config.Tariff, langCode string, tm *translation.Manager) string {
	// Разные эмодзи для разных тарифов
	emoji := "📱"
	switch tariff.Name {
	case "START":
		emoji = "⭐"
	case "PRO":
		emoji = "🚀"
	case "PREMIUM", "VIP":
		emoji = "💎"
	case "UNLIMITED":
		emoji = "♾️"
	}

	// Считаем среднемесячную цену от годовой подписки
	monthlyPrice := tariff.Price12 / 12

	return fmt.Sprintf("%s До %d устройств — от %d ₽/мес", emoji, tariff.Devices, monthlyPrice)
}

// TariffCallbackHandler обрабатывает выбор тарифа и показывает меню цен
func (h Handler) TariffCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	callback := update.CallbackQuery.Message.Message
	callbackQuery := parseCallbackData(update.CallbackQuery.Data)
	langCode := update.CallbackQuery.From.LanguageCode

	tariffName := callbackQuery["name"]
	if tariffName == "" {
		slog.Error("Tariff name not provided in callback")
		return
	}

	tariff := config.GetTariffByName(tariffName)
	if tariff == nil {
		slog.Error("Tariff not found", "name", tariffName)
		return
	}

	// Формируем кнопки с ценами выбранного тарифа
	var priceButtons []models.InlineKeyboardButton

	if tariff.Price1 > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_1", map[string]interface{}{"price": tariff.Price1}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d&tariff=%s", CallbackSell, 1, tariff.Price1, tariffName),
		})
	}

	if tariff.Price3 > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_3", map[string]interface{}{"price": tariff.Price3}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d&tariff=%s", CallbackSell, 3, tariff.Price3, tariffName),
		})
	}

	if tariff.Price6 > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_6", map[string]interface{}{"price": tariff.Price6}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d&tariff=%s", CallbackSell, 6, tariff.Price6, tariffName),
		})
	}

	if tariff.Price12 > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetTextTemplate(langCode, "month_12", map[string]interface{}{"price": tariff.Price12}),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d&tariff=%s", CallbackSell, 12, tariff.Price12, tariffName),
		})
	}

	keyboard := [][]models.InlineKeyboardButton{}

	if len(priceButtons) == 4 {
		keyboard = append(keyboard, priceButtons[:2])
		keyboard = append(keyboard, priceButtons[2:])
	} else if len(priceButtons) > 0 {
		keyboard = append(keyboard, priceButtons)
	}

	// Кнопка назад - к меню тарифов или к старту
	if len(config.GetTariffs()) > 1 {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackBuy},
		})
	} else {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart},
		})
	}

	// Пробуем отредактировать, если не получится (фото) — отправляем новое
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
		Text: h.translation.GetText(langCode, "pricing_info"),
	})

	if err != nil {
		// Игнорируем ошибки "message is not modified" (двойной клик)
		errStr := err.Error()
		if strings.Contains(errStr, "message is not modified") ||
			strings.Contains(errStr, "exactly the same") {
			return
		}
		// Если сообщение с фото — отправляем новое
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    callback.Chat.ID,
			ParseMode: models.ParseModeHTML,
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: keyboard,
			},
			Text: h.translation.GetText(langCode, "pricing_info"),
		})
	}
}
