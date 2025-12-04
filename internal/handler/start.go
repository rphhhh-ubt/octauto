package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/utils"
)

func (h Handler) StartCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	ctxWithTime, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	langCode := update.Message.From.LanguageCode
	existingCustomer, err := h.customerRepository.FindByTelegramId(ctx, update.Message.Chat.ID)
	if err != nil {
		slog.Error("error finding customer by telegram id", "error", err)
		return
	}

	if existingCustomer == nil {
		existingCustomer, err = h.customerRepository.Create(ctxWithTime, &database.Customer{
			TelegramID: update.Message.Chat.ID,
			Language:   langCode,
		})
		if err != nil {
			slog.Error("error creating customer", "error", err)
			return
		}

		if strings.Contains(update.Message.Text, "ref_") {
			arg := strings.Split(update.Message.Text, " ")[1]
			if strings.HasPrefix(arg, "ref_") {
				code := strings.TrimPrefix(arg, "ref_")
				referrerId, err := strconv.ParseInt(code, 10, 64)
				if err != nil {
					slog.Error("error parsing referrer id", "error", err)
					return
				}
				_, err = h.customerRepository.FindByTelegramId(ctx, referrerId)
				if err == nil {
					_, err := h.referralRepository.Create(ctx, referrerId, existingCustomer.TelegramID)
					if err != nil {
						slog.Error("error creating referral", "error", err)
						return
					}
					slog.Info("referral created", "referrerId", utils.MaskHalfInt64(referrerId), "refereeId", utils.MaskHalfInt64(existingCustomer.TelegramID))
				}
			}
		}
	}
	// Язык не обновляем — используем DEFAULT_LANGUAGE из конфига

	// Проверяем параметр deep link для перехода к тарифам
	if strings.Contains(update.Message.Text, "tariffs") || strings.Contains(update.Message.Text, "buy") {
		h.sendTariffsMenu(ctx, b, update.Message.Chat.ID, langCode)
		return
	}

	inlineKeyboard := h.buildStartKeyboard(existingCustomer, langCode)

	m, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "⚡",
		ReplyMarkup: models.ReplyKeyboardRemove{
			RemoveKeyboard: true,
		},
	})

	if err != nil {
		slog.Error("Error sending removing reply keyboard", "error", err)
		return
	}

	_, err = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    update.Message.Chat.ID,
		MessageID: m.ID,
	})

	if err != nil {
		slog.Error("Error deleting message", "error", err)
		return
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: inlineKeyboard,
		},
		Text: h.translation.GetText(langCode, "greeting"),
	})
	if err != nil {
		slog.Error("Error sending /start message", "error", err)
	}
}

// sendTariffsMenu отправляет меню тарифов напрямую (для deep link)
func (h Handler) sendTariffsMenu(ctx context.Context, b *bot.Bot, chatID int64, langCode string) {
	tariffs := config.GetTariffs()

	var keyboard [][]models.InlineKeyboardButton

	if len(tariffs) > 1 {
		// Несколько тарифов - показываем выбор
		for _, tariff := range tariffs {
			keyboard = append(keyboard, []models.InlineKeyboardButton{
				{
					Text:         FormatTariffButtonText(tariff, langCode, h.translation),
					CallbackData: fmt.Sprintf("%s?name=%s", CallbackTariff, tariff.Name),
				},
			})
		}
	} else if len(tariffs) == 1 {
		// Один тариф - показываем сразу цены
		tariff := tariffs[0]
		if tariff.Price1 > 0 {
			keyboard = append(keyboard, []models.InlineKeyboardButton{
				{Text: h.translation.GetTextTemplate(langCode, "month_1", map[string]interface{}{"price": tariff.Price1}),
					CallbackData: fmt.Sprintf("%s?month=%d&amount=%d&tariff=%s", CallbackSell, 1, tariff.Price1, tariff.Name)},
			})
		}
		if tariff.Price3 > 0 {
			keyboard = append(keyboard, []models.InlineKeyboardButton{
				{Text: h.translation.GetTextTemplate(langCode, "month_3", map[string]interface{}{"price": tariff.Price3}),
					CallbackData: fmt.Sprintf("%s?month=%d&amount=%d&tariff=%s", CallbackSell, 3, tariff.Price3, tariff.Name)},
			})
		}
		if tariff.Price6 > 0 {
			keyboard = append(keyboard, []models.InlineKeyboardButton{
				{Text: h.translation.GetTextTemplate(langCode, "month_6", map[string]interface{}{"price": tariff.Price6}),
					CallbackData: fmt.Sprintf("%s?month=%d&amount=%d&tariff=%s", CallbackSell, 6, tariff.Price6, tariff.Name)},
			})
		}
		if tariff.Price12 > 0 {
			keyboard = append(keyboard, []models.InlineKeyboardButton{
				{Text: h.translation.GetTextTemplate(langCode, "month_12", map[string]interface{}{"price": tariff.Price12}),
					CallbackData: fmt.Sprintf("%s?month=%d&amount=%d&tariff=%s", CallbackSell, 12, tariff.Price12, tariff.Name)},
			})
		}
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart},
	})

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
		Text: h.translation.GetText(langCode, "select_tariff"),
	})
	if err != nil {
		slog.Error("Error sending tariffs menu", "error", err)
	}
}

func (h Handler) StartCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	// Очищаем состояние ввода промокода при возврате в меню
	userID := update.CallbackQuery.From.ID
	h.cache.Delete(fmt.Sprintf("promo_state_%d", userID))

	ctxWithTime, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	callback := update.CallbackQuery
	langCode := callback.From.LanguageCode

	existingCustomer, err := h.customerRepository.FindByTelegramId(ctxWithTime, callback.From.ID)
	if err != nil {
		slog.Error("error finding customer by telegram id", "error", err)
		return
	}

	// Если customer не найден (удалён из БД) — создаём заново
	if existingCustomer == nil {
		existingCustomer, err = h.customerRepository.Create(ctxWithTime, &database.Customer{
			TelegramID: callback.From.ID,
			Language:   langCode,
		})
		if err != nil {
			slog.Error("error creating customer in callback", "error", err)
			return
		}
	}

	inlineKeyboard := h.buildStartKeyboard(existingCustomer, langCode)

	// Пробуем отредактировать, если не получится (фото) — отправляем новое
	_, err = b.EditMessageText(ctxWithTime, &bot.EditMessageTextParams{
		ChatID:    callback.Message.Message.Chat.ID,
		MessageID: callback.Message.Message.ID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: inlineKeyboard,
		},
		Text: h.translation.GetText(langCode, "greeting"),
	})
	if err != nil {
		// Игнорируем ошибки "message is not modified" (двойной клик)
		if strings.Contains(err.Error(), "message is not modified") ||
			strings.Contains(err.Error(), "exactly the same") {
			return
		}
		// Если сообщение с фото — отправляем новое
		_, _ = b.SendMessage(ctxWithTime, &bot.SendMessageParams{
			ChatID:    callback.Message.Message.Chat.ID,
			ParseMode: models.ParseModeHTML,
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: inlineKeyboard,
			},
			Text: h.translation.GetText(langCode, "greeting"),
		})
	}
}

func (h Handler) resolveConnectButton(lang string) []models.InlineKeyboardButton {
	var inlineKeyboard []models.InlineKeyboardButton

	if config.GetMiniAppURL() != "" {
		inlineKeyboard = []models.InlineKeyboardButton{
			{Text: h.translation.GetText(lang, "connect_button"), WebApp: &models.WebAppInfo{
				URL: config.GetMiniAppURL(),
			}},
		}
	} else {
		inlineKeyboard = []models.InlineKeyboardButton{
			{Text: h.translation.GetText(lang, "connect_button"), CallbackData: CallbackConnect},
		}
	}
	return inlineKeyboard
}

func (h Handler) buildStartKeyboard(existingCustomer *database.Customer, langCode string) [][]models.InlineKeyboardButton {
	var inlineKeyboard [][]models.InlineKeyboardButton

	if existingCustomer.SubscriptionLink == nil && config.TrialDays() > 0 {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{{Text: h.translation.GetText(langCode, "trial_button"), CallbackData: CallbackTrial}})
	}

	inlineKeyboard = append(inlineKeyboard, [][]models.InlineKeyboardButton{{{Text: h.translation.GetText(langCode, "buy_button"), CallbackData: CallbackBuy}}}...)

	if existingCustomer.SubscriptionLink != nil && existingCustomer.ExpireAt.After(time.Now()) {
		inlineKeyboard = append(inlineKeyboard, h.resolveConnectButton(langCode))
	}

	// Кнопка промокода
	inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{{Text: h.translation.GetText(langCode, "promo_button"), CallbackData: CallbackPromo}})

	if config.GetReferralDays() > 0 {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{{Text: h.translation.GetText(langCode, "referral_button"), CallbackData: CallbackReferral}})
	}

	if config.ServerStatusURL() != "" {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{{Text: h.translation.GetText(langCode, "server_status_button"), URL: config.ServerStatusURL()}})
	}

	if config.SupportURL() != "" {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{{Text: h.translation.GetText(langCode, "support_button"), URL: config.SupportURL()}})
	}

	if config.FeedbackURL() != "" {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{{Text: h.translation.GetText(langCode, "feedback_button"), URL: config.FeedbackURL()}})
	}

	if config.ChannelURL() != "" {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{{Text: h.translation.GetText(langCode, "channel_button"), URL: config.ChannelURL()}})
	}

	if config.TosURL() != "" {
		inlineKeyboard = append(inlineKeyboard, []models.InlineKeyboardButton{{Text: h.translation.GetText(langCode, "tos_button"), URL: config.TosURL()}})
	}
	return inlineKeyboard
}
