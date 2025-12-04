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

// AdminPromoTariffCallback показывает меню управления промокодами на тариф
// Requirements: 3.1
func (h Handler) AdminPromoTariffCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}

	// Clear any pending input states when returning to menu
	h.cache.Delete(fmt.Sprintf("admin_promo_state_%d", update.CallbackQuery.From.ID))
	h.cache.Delete(fmt.Sprintf("admin_promo_tariff_state_%d", update.CallbackQuery.From.ID))

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "➕ Создать промокод на тариф", CallbackData: "admin_promo_tariff_create"}},
			{{Text: "📋 Список промокодов на тариф", CallbackData: "admin_promo_tariff_list"}},
			{{Text: "🔙 Назад", CallbackData: "admin_promo"}},
		},
	}

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      update.CallbackQuery.Message.Message.Chat.ID,
		MessageID:   update.CallbackQuery.Message.Message.ID,
		Text:        "🎁 <b>Промокоды на тариф</b>\n\nПромокод на тариф сохраняет специальное предложение для пользователя (цена, устройства, период).\n\nВыберите действие:",
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
	if err != nil {
		slog.Error("Error editing promo tariff admin menu", "error", err)
	}
}

// AdminPromoTariffCreateCallback начинает процесс создания промокода на тариф
// Requirements: 2.1
func (h Handler) AdminPromoTariffCreateCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}

	// Clear conflicting state from regular promo handler
	conflictKey := fmt.Sprintf("admin_promo_state_%d", update.CallbackQuery.From.ID)
	h.cache.Delete(conflictKey)

	// Set state
	key := fmt.Sprintf("admin_promo_tariff_state_%d", update.CallbackQuery.From.ID)
	h.cache.SetString(key, "waiting_code", 600)

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "❌ Отмена", CallbackData: "admin_promo_tariff"}},
		},
	}

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    update.CallbackQuery.Message.Message.Chat.ID,
		MessageID: update.CallbackQuery.Message.Message.ID,
		Text: "➕ <b>Создание промокода на тариф</b>\n\n" +
			"Отправьте данные в формате:\n" +
			"<code>КОД ЦЕНА УСТРОЙСТВА МЕСЯЦЫ ЛИМИТ ЧАСЫ</code>\n\n" +
			"Пример: <code>NEWYEAR 199 3 1 100 48</code>\n" +
			"(промокод NEWYEAR, цена 199₽, 3 устройства, 1 месяц, лимит 100 активаций, предложение действует 48 часов)\n\n" +
			"Или с датой истечения промокода:\n" +
			"<code>КОД ЦЕНА УСТРОЙСТВА МЕСЯЦЫ ЛИМИТ ЧАСЫ ДАТА</code>\n" +
			"Пример: <code>WINTER 99 1 1 50 24 2025-12-31</code>",
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
	if err != nil {
		slog.Error("Error editing promo tariff create message", "error", err)
	}
}

// AdminPromoTariffCreateInputHandler обрабатывает ввод данных для создания промокода на тариф
// Requirements: 2.2, 2.3, 2.4
func (h Handler) AdminPromoTariffCreateInputHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.From.ID != config.GetAdminTelegramId() {
		return
	}

	userID := update.Message.From.ID
	chatID := update.Message.Chat.ID
	stateKey := fmt.Sprintf("admin_promo_tariff_state_%d", userID)

	state, found := h.cache.GetString(stateKey)
	if !found || state != "waiting_code" {
		return
	}

	// Хелпер для отправки ошибки с сохранением состояния
	sendError := func(text string) {
		h.cache.SetString(stateKey, "waiting_code", 600)
		keyboard := &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "❌ Отмена", CallbackData: "admin_promo_tariff"}},
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
	if len(parts) < 6 {
		sendError("❌ Неверный формат. Используйте: <code>КОД ЦЕНА УСТРОЙСТВА МЕСЯЦЫ ЛИМИТ ЧАСЫ [ДАТА]</code>")
		return
	}

	code := strings.ToUpper(parts[0])

	// Валидация кода: только буквы, цифры, подчёркивания и дефисы, 3-50 символов
	if len(code) < 3 || len(code) > 50 {
		sendError("❌ Код должен быть от 3 до 50 символов")
		return
	}
	for _, r := range code {
		if !((r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			sendError("❌ Код может содержать только латинские буквы, цифры, подчёркивания и дефисы")
			return
		}
	}

	price, err := strconv.Atoi(parts[1])
	if err != nil || price <= 0 {
		sendError("❌ Неверная цена (должно быть положительное число)")
		return
	}

	devices, err := strconv.Atoi(parts[2])
	if err != nil || devices <= 0 {
		sendError("❌ Неверное количество устройств (должно быть положительное число)")
		return
	}

	months, err := strconv.Atoi(parts[3])
	if err != nil || months <= 0 {
		sendError("❌ Неверное количество месяцев (должно быть положительное число)")
		return
	}
	if months > 12 {
		sendError("❌ Максимум 12 месяцев")
		return
	}

	maxActivations, err := strconv.Atoi(parts[4])
	if err != nil || maxActivations <= 0 {
		sendError("❌ Неверный лимит активаций (должно быть положительное число)")
		return
	}
	if maxActivations > 100000 {
		sendError("❌ Максимум 100000 активаций")
		return
	}

	validHours, err := strconv.Atoi(parts[5])
	if err != nil || validHours <= 0 {
		sendError("❌ Неверный срок действия предложения в часах (должно быть положительное число)")
		return
	}
	if validHours > 720 { // 30 дней
		sendError("❌ Максимум 720 часов (30 дней)")
		return
	}

	var validUntil *time.Time
	if len(parts) >= 7 {
		t, err := time.Parse("2006-01-02", parts[6])
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

	promo, err := h.promoTariffService.CreatePromoTariffCode(ctx, code, price, devices, months, maxActivations, validHours, userID, validUntil)
	if err != nil {
		errMsg := fmt.Sprintf("❌ Ошибка создания: %v", err)
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "exists") {
			errMsg = fmt.Sprintf("❌ Промокод <code>%s</code> уже существует", code)
		}
		h.cache.SetString(stateKey, "waiting_code", 600)
		keyboard := &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "❌ Отмена", CallbackData: "admin_promo_tariff"}},
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
			{{Text: "🔙 Назад", CallbackData: "admin_promo_tariff"}},
		},
	}

	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text: fmt.Sprintf(
			"✅ <b>Промокод на тариф создан!</b>\n\n"+
				"Код: <code>%s</code>\n"+
				"Цена: %d₽\n"+
				"Устройства: %d\n"+
				"Период: %d мес.\n"+
				"Лимит: %d активаций\n"+
				"Предложение действует: %d ч.\n"+
				"Промокод действует до: %s",
			promo.Code, promo.Price, promo.Devices, promo.Months, promo.MaxActivations, promo.ValidHours, validStr,
		),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
}


// AdminPromoTariffListCallback показывает список промокодов на тариф
// Requirements: 3.1
func (h Handler) AdminPromoTariffListCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}

	promos, err := h.promoTariffService.GetAllPromoTariffCodes(ctx, 20, 0)
	if err != nil {
		slog.Error("Error getting promo tariff list", "error", err)
		return
	}

	text := "📋 <b>Список промокодов на тариф</b>\n\nНажмите на промокод для управления:"

	var buttons [][]models.InlineKeyboardButton

	if len(promos) == 0 {
		text = "📋 <b>Список промокодов на тариф</b>\n\nПромокодов пока нет"
	} else {
		for _, p := range promos {
			status := "✅"
			if !p.IsActive {
				status = "❌"
			}
			// Формат: статус КОД (цена₽, устройства, месяцы) активации/лимит
			btnText := fmt.Sprintf("%s %s (%d₽, %dу, %dм) %d/%d",
				status, p.Code, p.Price, p.Devices, p.Months, p.CurrentActivations, p.MaxActivations)
			buttons = append(buttons, []models.InlineKeyboardButton{
				{Text: btnText, CallbackData: fmt.Sprintf("admin_promo_tariff_view_%d", p.ID)},
			})
		}
	}

	buttons = append(buttons, []models.InlineKeyboardButton{{Text: "🔙 Назад", CallbackData: "admin_promo_tariff"}})

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
		slog.Error("Error editing promo tariff list", "error", err)
	}

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
}

// AdminPromoTariffViewCallback показывает детали промокода на тариф
// Requirements: 3.2, 3.3
func (h Handler) AdminPromoTariffViewCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}

	idStr := strings.TrimPrefix(update.CallbackQuery.Data, "admin_promo_tariff_view_")
	promoID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return
	}

	promo, err := h.promoTariffService.GetPromoTariffByID(ctx, promoID)
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
		"🎁 <b>Промокод на тариф: %s</b>\n\n"+
			"Статус: %s\n"+
			"Цена: %d₽\n"+
			"Устройства: %d\n"+
			"Период: %d мес.\n"+
			"Активаций: %d/%d\n"+
			"Предложение действует: %d ч.\n"+
			"Промокод действует до: %s\n"+
			"Создан: %s",
		promo.Code, status, promo.Price, promo.Devices, promo.Months,
		promo.CurrentActivations, promo.MaxActivations, promo.ValidHours,
		validStr, promo.CreatedAt.Format("02.01.2006 15:04"),
	)

	var buttons [][]models.InlineKeyboardButton
	if promo.IsActive {
		buttons = append(buttons, []models.InlineKeyboardButton{
			{Text: "⏸ Деактивировать", CallbackData: fmt.Sprintf("admin_promo_tariff_deactivate_%d", promo.ID)},
		})
	} else {
		buttons = append(buttons, []models.InlineKeyboardButton{
			{Text: "▶️ Активировать", CallbackData: fmt.Sprintf("admin_promo_tariff_activate_%d", promo.ID)},
		})
	}
	buttons = append(buttons, []models.InlineKeyboardButton{
		{Text: "🗑 Удалить", CallbackData: fmt.Sprintf("admin_promo_tariff_delete_%d", promo.ID)},
	})
	buttons = append(buttons, []models.InlineKeyboardButton{
		{Text: "🔙 К списку", CallbackData: "admin_promo_tariff_list"},
	})

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

// AdminPromoTariffDeleteCallback удаляет промокод на тариф
// Requirements: 3.3
func (h Handler) AdminPromoTariffDeleteCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}

	idStr := strings.TrimPrefix(update.CallbackQuery.Data, "admin_promo_tariff_delete_")
	promoID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return
	}

	err = h.promoTariffService.DeletePromoTariff(ctx, promoID)
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
	h.AdminPromoTariffListCallback(ctx, b, update)
}

// AdminPromoTariffToggleCallback активирует/деактивирует промокод на тариф
// Requirements: 3.2
func (h Handler) AdminPromoTariffToggleCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		return
	}

	data := update.CallbackQuery.Data
	var promoID int64
	var activate bool

	if strings.HasPrefix(data, "admin_promo_tariff_activate_") {
		idStr := strings.TrimPrefix(data, "admin_promo_tariff_activate_")
		promoID, _ = strconv.ParseInt(idStr, 10, 64)
		activate = true
	} else if strings.HasPrefix(data, "admin_promo_tariff_deactivate_") {
		idStr := strings.TrimPrefix(data, "admin_promo_tariff_deactivate_")
		promoID, _ = strconv.ParseInt(idStr, 10, 64)
		activate = false
	}

	var err error
	if activate {
		err = h.promoTariffService.ActivatePromoTariff(ctx, promoID)
	} else {
		err = h.promoTariffService.DeactivatePromoTariff(ctx, promoID)
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
	update.CallbackQuery.Data = fmt.Sprintf("admin_promo_tariff_view_%d", promoID)
	h.AdminPromoTariffViewCallback(ctx, b, update)
}

// PromoTariffCallbackHandler обрабатывает нажатие на кнопку promo tariff в меню тарифов
// Показывает кнопки оплаты с ценой из promo offer (аналогично winback)
// Requirements: 5.3
func (h Handler) PromoTariffCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	callback := update.CallbackQuery.Message.Message
	langCode := update.CallbackQuery.From.LanguageCode
	telegramID := update.CallbackQuery.From.ID

	// Get customer
	customer, err := h.customerRepository.FindByTelegramId(ctx, telegramID)
	if err != nil {
		slog.Error("Error finding customer for promo tariff", "error", err)
		return
	}
	if customer == nil {
		slog.Error("Customer not found for promo tariff")
		return
	}

	// Check if customer has active promo offer
	if !HasActivePromoOffer(customer) {
		slog.Warn("No active promo offer for customer", "customerID", customer.ID)
		h.sendPromoTariffError(ctx, b, callback, langCode, "promo_tariff_offer_expired")
		return
	}

	// Get offer parameters
	price := customer.PromoOfferPrice
	months := customer.PromoOfferMonths

	if price == nil || months == nil {
		slog.Error("Promo offer has nil parameters", "customerID", customer.ID)
		h.sendPromoTariffError(ctx, b, callback, langCode, "promo_tariff_error")
		return
	}

	slog.Info("Showing promo tariff payment options",
		"customerID", customer.ID,
		"price", *price,
		"months", *months)

	// Show payment options (like winback)
	h.showPromoTariffPaymentOptions(ctx, b, callback, langCode, *price, *months)
}

// HasActivePromoOffer проверяет, есть ли у пользователя активное promo tariff предложение
// Property 7: Offer Visibility Based on Expiration
func HasActivePromoOffer(customer *database.Customer) bool {
	if customer == nil {
		return false
	}
	if customer.PromoOfferPrice == nil || customer.PromoOfferExpiresAt == nil {
		return false
	}
	return customer.PromoOfferExpiresAt.After(time.Now())
}

// showPromoTariffPaymentOptions показывает кнопки оплаты для promo tariff предложения
// Аналогично winback, но с пометкой promo_tariff
func (h Handler) showPromoTariffPaymentOptions(ctx context.Context, b *bot.Bot, callback *models.Message, langCode string, price int, months int) {
	// Build payment callback with promo_tariff flag (short keys for 64 byte limit)
	buildPaymentCallback := func(invoiceType database.InvoiceType) string {
		return fmt.Sprintf("%s?m=%d&t=%s&a=%d&pt=1", CallbackPayment, months, invoiceType, price)
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


	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackBuy},
	})

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		Text:      h.translation.GetText(langCode, "promo_tariff_select_payment"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
	})

	if err != nil {
		slog.Error("Error showing promo tariff payment options", "error", err)
	}
}

// sendPromoTariffError отправляет сообщение об ошибке
func (h Handler) sendPromoTariffError(ctx context.Context, b *bot.Bot, callback *models.Message, langCode string, errorKey string) {
	text := h.translation.GetText(langCode, errorKey)
	if text == "" {
		text = h.translation.GetText(langCode, "promo_tariff_error")
	}

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "buy_button"), CallbackData: CallbackBuy}},
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart}},
			},
		},
	})
	if err != nil {
		slog.Error("Error sending promo tariff error message", "error", err)
	}
}
