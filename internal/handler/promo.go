package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)



// User handler - apply promo code (из главного меню — редактирует сообщение)
func (h Handler) PromoCodeCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	lang := update.CallbackQuery.From.LanguageCode
	callback := update.CallbackQuery.Message.Message
	chatID := callback.Chat.ID

	// Set state to wait for promo code input
	key := fmt.Sprintf("promo_state_%d", update.CallbackQuery.From.ID)
	h.cache.SetString(key, "waiting_code", 300) // 5 minutes

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: h.translation.GetText(lang, "back_to_menu"), CallbackData: CallbackStart}},
		},
	}

	// Редактируем сообщение
	_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   callback.ID,
		Text:        h.translation.GetText(lang, "promo_enter_code"),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
}

// BroadcastPromoCallbackHandler - обработчик кнопки промокода из broadcast (всегда новое сообщение)
func (h Handler) BroadcastPromoCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	lang := update.CallbackQuery.From.LanguageCode
	chatID := update.CallbackQuery.Message.Message.Chat.ID

	// Set state to wait for promo code input
	key := fmt.Sprintf("promo_state_%d", update.CallbackQuery.From.ID)
	h.cache.SetString(key, "waiting_code", 300) // 5 minutes

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: h.translation.GetText(lang, "back_to_menu"), CallbackData: CallbackStart}},
		},
	}

	// Всегда новое сообщение чтобы не терять broadcast
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        h.translation.GetText(lang, "promo_enter_code"),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
}

// Handle promo code text input
// Requirements: 4.1, 4.2, 4.6, 7.1, 7.2
func (h Handler) PromoCodeInputHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	userID := update.Message.From.ID
	stateKey := fmt.Sprintf("promo_state_%d", userID)
	
	state, found := h.cache.GetString(stateKey)
	if !found || state != "waiting_code" {
		return
	}

	// Clear state
	h.cache.Delete(stateKey)

	lang := update.Message.From.LanguageCode
	chatID := update.Message.Chat.ID
	code := strings.TrimSpace(update.Message.Text)

	// Get customer
	customer, err := h.customerRepository.FindByTelegramId(ctx, userID)
	if err != nil || customer == nil {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   h.translation.GetText(lang, "error_occurred"),
		})
		return
	}

	// First try promo tariff code if feature is enabled
	// Requirements: 4.6 - backward compatibility with regular promo codes
	if config.IsPromoTariffCodesEnabled() {
		tariffResult := h.promoTariffService.ApplyPromoTariffCode(ctx, customer.ID, code)
		
		// If promo tariff code found (success or specific error), handle it
		if tariffResult.Success || (tariffResult.ErrorKey != "promo_tariff_not_found" && tariffResult.ErrorKey != "promo_tariff_invalid_format") {
			if !tariffResult.Success {
				// Promo tariff code found but validation failed
				h.cache.SetString(stateKey, "waiting_code", 300)
				
				keyboard := &models.InlineKeyboardMarkup{
					InlineKeyboard: [][]models.InlineKeyboardButton{
						{{Text: h.translation.GetText(lang, "back_to_menu"), CallbackData: CallbackStart}},
					},
				}
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
					ChatID:      chatID,
					Text:        h.translation.GetText(lang, tariffResult.ErrorKey) + "\n\n" + h.translation.GetText(lang, "promo_try_again"),
					ParseMode:   models.ParseModeHTML,
					ReplyMarkup: keyboard,
				})
				return
			}

			// Success - promo tariff code activated
			slog.Info("Promo tariff code activated",
				"customerID", customer.ID,
				"code", code)

			// Получаем обновлённые данные customer с promo offer
			updatedCustomer, err := h.customerRepository.FindByTelegramId(ctx, userID)
			if err != nil || updatedCustomer == nil {
				slog.Error("Error getting updated customer after promo tariff activation", "error", err)
				return
			}

			// Показываем сообщение с информацией о тарифе
			h.sendPromoTariffActivatedMessage(ctx, b, chatID, lang, updatedCustomer, tariffResult.OfferExpires)
			return
		}
		// If not found or invalid format, fall through to regular promo codes
	}

	// Apply regular promo code (backward compatibility)
	ctxWithUsername := context.WithValue(ctx, "username", update.Message.From.Username)
	result := h.promoService.ApplyPromoCode(ctxWithUsername, customer.ID, userID, code)

	if !result.Success {
		// Восстанавливаем состояние для повторного ввода
		h.cache.SetString(stateKey, "waiting_code", 300)
		
		keyboard := &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(lang, "back_to_menu"), CallbackData: CallbackStart}},
			},
		}
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      chatID,
			Text:        h.translation.GetText(lang, result.ErrorKey) + "\n\n" + h.translation.GetText(lang, "promo_try_again"),
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: keyboard,
		})
		return
	}

	// Success message
	expireStr := ""
	if result.NewExpire != nil {
		expireStr = result.NewExpire.Format("02.01.2006")
	}

	text := h.translation.GetTextTemplate(lang, "promo_success", map[string]interface{}{
		"days":      result.BonusDays,
		"expire_at": expireStr,
	})

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: h.translation.GetText(lang, "back_to_menu"), CallbackData: CallbackStart}},
		},
	}

	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
}

// sendPromoTariffActivatedMessage отправляет сообщение об успешной активации промокода на тариф
// Показывает характеристики тарифа и кнопку активации
func (h Handler) sendPromoTariffActivatedMessage(ctx context.Context, b *bot.Bot, chatID int64, langCode string, customer *database.Customer, expiresAt *time.Time) {
	if customer == nil || customer.PromoOfferPrice == nil || customer.PromoOfferMonths == nil || customer.PromoOfferDevices == nil {
		slog.Error("Invalid promo offer data")
		return
	}

	price := *customer.PromoOfferPrice
	months := *customer.PromoOfferMonths
	devices := *customer.PromoOfferDevices

	// Форматируем срок действия
	expiresStr := ""
	if expiresAt != nil {
		expiresStr = expiresAt.Format("02.01.2006 15:04")
	}

	// Форматируем период
	monthsWord := "месяц"
	if months >= 2 && months <= 4 {
		monthsWord = "месяца"
	} else if months >= 5 {
		monthsWord = "месяцев"
	}

	// Форматируем устройства
	devicesWord := "устройство"
	if devices >= 2 && devices <= 4 {
		devicesWord = "устройства"
	} else if devices >= 5 {
		devicesWord = "устройств"
	}

	// Формируем текст сообщения
	text := fmt.Sprintf(
		"✅ <b>Промокод активирован!</b>\n\n"+
			"🎁 <b>Вам доступен специальный тариф:</b>\n\n"+
			"💰 Цена: <b>%d₽</b>\n"+
			"📅 Период: <b>%d %s</b>\n"+
			"📱 Устройств: <b>%d %s</b>\n\n"+
			"⏰ Предложение действует до: <b>%s</b>",
		price, months, monthsWord, devices, devicesWord, expiresStr,
	)

	keyboard := [][]models.InlineKeyboardButton{
		{{Text: "🎁 Активировать тариф", CallbackData: CallbackPromoTariff}},
		{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart}},
	}

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
		Text: text,
	})
	if err != nil {
		slog.Error("Error sending promo tariff activated message", "error", err)
	}
}

// Admin handlers

func (h Handler) AdminPromoCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}

	// Clear any pending input states when returning to menu
	h.cache.Delete(fmt.Sprintf("admin_promo_state_%d", update.CallbackQuery.From.ID))
	h.cache.Delete(fmt.Sprintf("admin_promo_tariff_state_%d", update.CallbackQuery.From.ID))

	buttons := [][]models.InlineKeyboardButton{
		{{Text: "➕ Создать промокод", CallbackData: "admin_promo_create"}},
		{{Text: "📋 Список промокодов", CallbackData: "admin_promo_list"}},
	}

	// Добавляем кнопку промокодов на тариф если функция включена
	if config.IsPromoTariffCodesEnabled() {
		buttons = append(buttons, []models.InlineKeyboardButton{
			{Text: "🎁 Промокод на тариф", CallbackData: "admin_promo_tariff"},
		})
	}

	buttons = append(buttons, []models.InlineKeyboardButton{
		{Text: "🔙 Назад", CallbackData: "admin_back"},
	})

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: buttons,
	}

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      update.CallbackQuery.Message.Message.Chat.ID,
		MessageID:   update.CallbackQuery.Message.Message.ID,
		Text:        "🎟 <b>Управление промокодами</b>\n\nВыберите действие:",
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
	if err != nil {
		slog.Error("Error editing promo admin menu", "error", err)
	}

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
}

func (h Handler) AdminPromoCreateCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}

	// Clear conflicting state from promo tariff handler
	conflictKey := fmt.Sprintf("admin_promo_tariff_state_%d", update.CallbackQuery.From.ID)
	h.cache.Delete(conflictKey)

	// Set state
	key := fmt.Sprintf("admin_promo_state_%d", update.CallbackQuery.From.ID)
	h.cache.SetString(key, "waiting_code", 600)

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "❌ Отмена", CallbackData: "admin_promo"}},
		},
	}

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    update.CallbackQuery.Message.Message.Chat.ID,
		MessageID: update.CallbackQuery.Message.Message.ID,
		Text: "➕ <b>Создание промокода</b>\n\n" +
			"Отправьте данные в формате:\n" +
			"<code>КОД ДНЕЙ ЛИМИТ</code>\n\n" +
			"Пример: <code>NEWYEAR2025 30 100</code>\n" +
			"(промокод NEWYEAR2025 на 30 дней, лимит 100 активаций)\n\n" +
			"Или с датой истечения:\n" +
			"<code>КОД ДНЕЙ ЛИМИТ ДАТА</code>\n" +
			"Пример: <code>WINTER 7 50 2025-12-31</code>",
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
	if err != nil {
		slog.Error("Error editing promo create message", "error", err)
	}

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
}

func (h Handler) AdminPromoCreateInputHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.From.ID != config.GetAdminTelegramId() {
		return
	}

	userID := update.Message.From.ID
	chatID := update.Message.Chat.ID
	stateKey := fmt.Sprintf("admin_promo_state_%d", userID)
	
	state, found := h.cache.GetString(stateKey)
	if !found || state != "waiting_code" {
		return
	}

	// Хелпер для отправки ошибки с сохранением состояния
	sendError := func(text string) {
		h.cache.SetString(stateKey, "waiting_code", 600) // восстанавливаем состояние
		keyboard := &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "❌ Отмена", CallbackData: "admin_promo"}},
			},
		}
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      chatID,
			Text:        text + "\n\nПопробуйте ещё раз или нажмите Отмена.",
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: keyboard,
		})
	}

	parts := strings.Fields(update.Message.Text)
	if len(parts) < 3 {
		sendError("❌ Неверный формат. Используйте: <code>КОД ДНЕЙ ЛИМИТ [ДАТА]</code>")
		return
	}

	code := strings.ToUpper(parts[0])
	
	// Валидация кода: только буквы, цифры и подчёркивания, 3-20 символов
	if len(code) < 3 || len(code) > 20 {
		sendError("❌ Код должен быть от 3 до 20 символов")
		return
	}
	for _, r := range code {
		if !((r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			sendError("❌ Код может содержать только латинские буквы, цифры и подчёркивания")
			return
		}
	}

	days, err := strconv.Atoi(parts[1])
	if err != nil || days <= 0 {
		sendError("❌ Неверное количество дней (должно быть положительное число)")
		return
	}
	if days > 365 {
		sendError("❌ Максимум 365 дней")
		return
	}

	limit, err := strconv.Atoi(parts[2])
	if err != nil || limit <= 0 {
		sendError("❌ Неверный лимит активаций (должно быть положительное число)")
		return
	}
	if limit > 100000 {
		sendError("❌ Максимум 100000 активаций")
		return
	}

	var validUntil *time.Time
	if len(parts) >= 4 {
		t, err := time.Parse("2006-01-02", parts[3])
		if err != nil {
			sendError("❌ Неверный формат даты. Используйте: <code>ГГГГ-ММ-ДД</code> (например: 2025-12-31)")
			return
		}
		if t.Before(time.Now()) {
			sendError("❌ Дата истечения должна быть в будущем")
			return
		}
		validUntil = &t
	}

	// Очищаем состояние только после успешной валидации
	h.cache.Delete(stateKey)

	_, err = h.promoService.CreatePromoCode(ctx, code, days, limit, userID, validUntil)
	if err != nil {
		errMsg := fmt.Sprintf("❌ Ошибка создания: %v", err)
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			errMsg = fmt.Sprintf("❌ Промокод <code>%s</code> уже существует", code)
		}
		h.cache.SetString(stateKey, "waiting_code", 600)
		keyboard := &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "❌ Отмена", CallbackData: "admin_promo"}},
			},
		}
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      chatID,
			Text:        errMsg + "\n\nПопробуйте ещё раз или нажмите Отмена.",
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: keyboard,
		})
		return
	}

	validStr := "без ограничения"
	if validUntil != nil {
		validStr = validUntil.Format("02.01.2006")
	}

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "🔙 Назад", CallbackData: "admin_promo"}},
		},
	}

	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text: fmt.Sprintf(
			"✅ <b>Промокод создан!</b>\n\n"+
				"Код: <code>%s</code>\n"+
				"Бонус: %d дней\n"+
				"Лимит: %d активаций\n"+
				"Действует до: %s",
			code, days, limit, validStr,
		),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
}

func (h Handler) AdminPromoListCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}

	promos, err := h.promoService.GetAllPromoCodes(ctx, 20, 0)
	if err != nil {
		slog.Error("Error getting promo list", "error", err)
		return
	}

	text := "📋 <b>Список промокодов</b>\n\nНажмите на промокод для управления:"

	var buttons [][]models.InlineKeyboardButton

	if len(promos) == 0 {
		text = "📋 <b>Список промокодов</b>\n\nПромокодов пока нет"
	} else {
		for _, p := range promos {
			status := "✅"
			if !p.IsActive {
				status = "❌"
			}
			btnText := fmt.Sprintf("%s %s (+%d дн, %d/%d)", status, p.Code, p.BonusDays, p.CurrentActivations, p.MaxActivations)
			buttons = append(buttons, []models.InlineKeyboardButton{
				{Text: btnText, CallbackData: fmt.Sprintf("admin_promo_view_%d", p.ID)},
			})
		}
	}

	buttons = append(buttons, []models.InlineKeyboardButton{{Text: "🔙 Назад", CallbackData: "admin_promo"}})

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: buttons,
	}

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      update.CallbackQuery.Message.Message.Chat.ID,
		MessageID:   update.CallbackQuery.Message.Message.ID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
	if err != nil {
		slog.Error("Error editing promo list", "error", err)
	}

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
}

func (h Handler) AdminPromoViewCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}

	idStr := strings.TrimPrefix(update.CallbackQuery.Data, "admin_promo_view_")
	promoID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return
	}

	promo, err := h.promoService.GetPromoByID(ctx, promoID)
	if err != nil || promo == nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Промокод не найден",
			ShowAlert:       true,
		})
		return
	}

	status := "✅ Активен"
	if !promo.IsActive {
		status = "❌ Неактивен"
	}
	validStr := "без ограничения"
	if promo.ValidUntil != nil {
		validStr = promo.ValidUntil.Format("02.01.2006")
	}

	text := fmt.Sprintf(
		"🎟 <b>Промокод: %s</b>\n\n"+
			"Статус: %s\n"+
			"Бонус: +%d дней\n"+
			"Активаций: %d/%d\n"+
			"Действует до: %s\n"+
			"Создан: %s",
		promo.Code, status, promo.BonusDays, promo.CurrentActivations, promo.MaxActivations, validStr, promo.CreatedAt.Format("02.01.2006 15:04"),
	)

	var buttons [][]models.InlineKeyboardButton
	if promo.IsActive {
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: "⏸ Деактивировать", CallbackData: fmt.Sprintf("admin_promo_deactivate_%d", promo.ID)}})
	} else {
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: "▶️ Активировать", CallbackData: fmt.Sprintf("admin_promo_activate_%d", promo.ID)}})
	}
	buttons = append(buttons, []models.InlineKeyboardButton{{Text: "🗑 Удалить", CallbackData: fmt.Sprintf("admin_promo_delete_%d", promo.ID)}})
	buttons = append(buttons, []models.InlineKeyboardButton{{Text: "🔙 К списку", CallbackData: "admin_promo_list"}})

	keyboard := &models.InlineKeyboardMarkup{InlineKeyboard: buttons}

	_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      update.CallbackQuery.Message.Message.Chat.ID,
		MessageID:   update.CallbackQuery.Message.Message.ID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: update.CallbackQuery.ID})
}

func (h Handler) AdminPromoDeleteCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}

	idStr := strings.TrimPrefix(update.CallbackQuery.Data, "admin_promo_delete_")
	promoID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return
	}

	err = h.promoService.DeletePromo(ctx, promoID)
	if err != nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Ошибка удаления",
			ShowAlert:       true,
		})
		return
	}

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		Text:            "✅ Промокод удалён",
	})

	// Возвращаемся к списку
	h.AdminPromoListCallback(ctx, b, update)
}

func (h Handler) AdminPromoToggleCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}

	data := update.CallbackQuery.Data
	var promoID int64
	var activate bool

	if strings.HasPrefix(data, "admin_promo_activate_") {
		idStr := strings.TrimPrefix(data, "admin_promo_activate_")
		promoID, _ = strconv.ParseInt(idStr, 10, 64)
		activate = true
	} else if strings.HasPrefix(data, "admin_promo_deactivate_") {
		idStr := strings.TrimPrefix(data, "admin_promo_deactivate_")
		promoID, _ = strconv.ParseInt(idStr, 10, 64)
		activate = false
	}

	var err error
	if activate {
		err = h.promoService.ActivatePromo(ctx, promoID)
	} else {
		err = h.promoService.DeactivatePromo(ctx, promoID)
	}

	if err != nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Ошибка",
			ShowAlert:       true,
		})
		return
	}

	msg := "✅ Деактивирован"
	if activate {
		msg = "✅ Активирован"
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		Text:            msg,
	})

	// Обновляем view
	update.CallbackQuery.Data = fmt.Sprintf("admin_promo_view_%d", promoID)
	h.AdminPromoViewCallback(ctx, b, update)
}
