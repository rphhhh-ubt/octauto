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

	"remnawave-tg-shop-bot/internal/broadcast"
	"remnawave-tg-shop-bot/internal/config"
)

func (h Handler) AdminCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message.From.ID != config.GetAdminTelegramId() {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   h.translation.GetText(update.Message.From.LanguageCode, "access_denied"),
		})
		return
	}

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "🎟 Промокоды", CallbackData: "admin_promo"},
			},
			{
				{Text: "📨 Рассылка", CallbackData: "admin_broadcast"},
			},
			{
				{Text: "📊 История рассылок", CallbackData: "admin_broadcast_history"},
			},
			{
				{Text: "🧪 Тест уведомлений", CallbackData: "admin_test_notifications"},
			},
			{
				{Text: "❌ Закрыть", CallbackData: "admin_close"},
			},
		},
	}

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      update.Message.Chat.ID,
		Text:        "🔧 <b>Панель администратора</b>\n\nВыберите действие:",
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
	if err != nil {
		slog.Error("Error sending admin menu", "error", err)
	}
}

func (h Handler) AdminBroadcastCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Доступ запрещён",
			ShowAlert:       true,
		})
		return
	}

	// Очищаем состояния рассылки при возврате в меню
	userID := update.CallbackQuery.From.ID
	h.cache.Delete(fmt.Sprintf("broadcast_state_%d", userID))
	h.cache.Delete(fmt.Sprintf("broadcast_target_%d", userID))

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "👥 Всем пользователям", CallbackData: "broadcast_target_all"},
			},
			{
				{Text: "✅ С подпиской", CallbackData: "broadcast_target_with_subscription"},
			},
			{
				{Text: "❌ Без подписки", CallbackData: "broadcast_target_without_subscription"},
			},
			{
				{Text: "⏰ С истекающей подпиской", CallbackData: "broadcast_target_expiring"},
			},
			{
				{Text: "👋 Только нажали /start", CallbackData: "broadcast_target_start_only"},
			},
			{
				{Text: "🔙 Назад", CallbackData: "admin_back"},
			},
		},
	}

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      update.CallbackQuery.Message.Message.Chat.ID,
		MessageID:   update.CallbackQuery.Message.Message.ID,
		Text:        "📨 <b>Выбор аудитории для рассылки</b>\n\nВыберите целевую группу:",
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
	if err != nil {
		slog.Error("Error editing message", "error", err)
	}

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
}

func (h Handler) AdminBroadcastTargetCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Доступ запрещён",
			ShowAlert:       true,
		})
		return
	}

	targetType := strings.TrimPrefix(update.CallbackQuery.Data, "broadcast_target_")
	userID := update.CallbackQuery.From.ID

	// Очищаем предыдущие данные рассылки
	h.cache.Delete(fmt.Sprintf("broadcast_media_%d", userID))
	h.cache.Delete(fmt.Sprintf("broadcast_media_type_%d", userID))
	h.cache.Delete(fmt.Sprintf("broadcast_text_%d", userID))
	h.cache.Delete(fmt.Sprintf("broadcast_buttons_%d", userID))

	// Сохраняем выбор в кеш для следующего шага
	key := fmt.Sprintf("broadcast_target_%d", userID)
	h.cache.SetString(key, targetType, 600) // 10 минут

	targetName := getTargetName(targetType)

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "🔙 Назад", CallbackData: "admin_broadcast"},
			},
		},
	}

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    update.CallbackQuery.Message.Message.Chat.ID,
		MessageID: update.CallbackQuery.Message.Message.ID,
		Text: fmt.Sprintf(
			"📝 <b>Введите текст сообщения</b>\n\n"+
				"Целевая аудитория: %s\n\n"+
				"Отправьте текст, фото, GIF, видео или кружок для рассылки.\n"+
				"Поддерживается HTML разметка.",
			targetName,
		),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
	if err != nil {
		slog.Error("Error editing message", "error", err)
	}

	// Сохраняем состояние ожидания сообщения
	stateKey := fmt.Sprintf("broadcast_state_%d", userID)
	h.cache.SetString(stateKey, "waiting_message", 600)

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
}

func (h Handler) AdminBroadcastMessageHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message.From.ID != config.GetAdminTelegramId() {
		return
	}

	userID := update.Message.From.ID
	stateKey := fmt.Sprintf("broadcast_state_%d", userID)
	state, found := h.cache.GetString(stateKey)
	if !found || state != "waiting_message" {
		return
	}

	targetKey := fmt.Sprintf("broadcast_target_%d", userID)
	targetType, found := h.cache.GetString(targetKey)
	if !found {
		return
	}

	// Получаем текст и/или медиа (фото, гиф, видео)
	var messageText string
	var mediaFileID string
	var mediaType string

	if update.Message.Photo != nil && len(update.Message.Photo) > 0 {
		// Фото - берем максимальный размер
		mediaFileID = update.Message.Photo[len(update.Message.Photo)-1].FileID
		mediaType = broadcast.MediaTypePhoto
		messageText = update.Message.Caption
	} else if update.Message.Animation != nil {
		// GIF/Animation
		mediaFileID = update.Message.Animation.FileID
		mediaType = broadcast.MediaTypeGIF
		messageText = update.Message.Caption
	} else if update.Message.Video != nil {
		// Видео
		mediaFileID = update.Message.Video.FileID
		mediaType = broadcast.MediaTypeVideo
		messageText = update.Message.Caption
	} else if update.Message.VideoNote != nil {
		// Кружок (видео-заметка)
		mediaFileID = update.Message.VideoNote.FileID
		mediaType = broadcast.MediaTypeVideoNote
		// VideoNote не поддерживает caption
	} else {
		messageText = update.Message.Text
	}

	if messageText == "" && mediaFileID == "" {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Отправьте текст, фото, GIF или видео",
		})
		return
	}

	// Сохраняем данные в кеш
	h.cache.SetString(fmt.Sprintf("broadcast_text_%d", userID), messageText, 600)
	if mediaFileID != "" {
		h.cache.SetString(fmt.Sprintf("broadcast_media_%d", userID), mediaFileID, 600)
		h.cache.SetString(fmt.Sprintf("broadcast_media_type_%d", userID), mediaType, 600)
	}

	// Переходим к выбору кнопок
	h.cache.SetString(stateKey, "waiting_buttons", 600)

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "🎟 Промокод", CallbackData: "broadcast_btn_promo"},
				{Text: "📱 Подписка", CallbackData: "broadcast_btn_subscription"},
			},
			{
				{Text: "💳 Купить", CallbackData: "broadcast_btn_buy"},
			},
			{
				{Text: "✅ Без кнопок / Готово", CallbackData: "broadcast_btn_done"},
			},
			{
				{Text: "🔙 Назад", CallbackData: "admin_broadcast"},
			},
		},
	}

	targetName := getTargetName(targetType)
	mediaInfo := getMediaInfo(mediaType)

	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text: fmt.Sprintf(
			"🔘 <b>Выберите кнопки для рассылки</b>\n\n"+
				"Целевая аудитория: %s%s\n\n"+
				"<b>Текст:</b>\n%s\n\n"+
				"Нажмите на кнопки которые хотите добавить, затем \"Готово\".",
			targetName,
			mediaInfo,
			messageText,
		),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
}

// getMediaInfo возвращает информацию о типе медиа для отображения
func getMediaInfo(mediaType string) string {
	switch mediaType {
	case broadcast.MediaTypePhoto:
		return "\n📷 Медиа: фото"
	case broadcast.MediaTypeGIF:
		return "\n🎬 Медиа: GIF"
	case broadcast.MediaTypeVideo:
		return "\n🎥 Медиа: видео"
	case broadcast.MediaTypeVideoNote:
		return "\n⭕ Медиа: кружок"
	default:
		return ""
	}
}

func (h Handler) AdminBroadcastButtonCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Доступ запрещён",
			ShowAlert:       true,
		})
		return
	}

	userID := update.CallbackQuery.From.ID
	data := update.CallbackQuery.Data

	// Получаем текущие выбранные кнопки
	buttonsKey := fmt.Sprintf("broadcast_buttons_%d", userID)
	currentButtons, _ := h.cache.GetString(buttonsKey)
	buttonsList := []string{}
	if currentButtons != "" {
		buttonsList = strings.Split(currentButtons, ",")
	}

	if data == "broadcast_btn_done" {
		// Переходим к подтверждению
		h.showBroadcastConfirmation(ctx, b, update)
		return
	}

	// Определяем какую кнопку добавить/убрать
	var btnName string
	switch data {
	case "broadcast_btn_promo":
		btnName = "promo"
	case "broadcast_btn_subscription":
		btnName = "subscription"
	case "broadcast_btn_buy":
		btnName = "buy"
	}

	// Toggle кнопки
	found := false
	newButtons := []string{}
	for _, btn := range buttonsList {
		if btn == btnName {
			found = true
			continue // убираем
		}
		newButtons = append(newButtons, btn)
	}
	if !found {
		newButtons = append(newButtons, btnName)
	}

	// Сохраняем
	h.cache.SetString(buttonsKey, strings.Join(newButtons, ","), 600)

	// Обновляем клавиатуру с отметками
	keyboard := h.buildBroadcastButtonsKeyboard(newButtons)

	targetKey := fmt.Sprintf("broadcast_target_%d", userID)
	targetType, _ := h.cache.GetString(targetKey)
	targetName := getTargetName(targetType)

	textKey := fmt.Sprintf("broadcast_text_%d", userID)
	messageText, _ := h.cache.GetString(textKey)

	mediaTypeKey := fmt.Sprintf("broadcast_media_type_%d", userID)
	mediaType, _ := h.cache.GetString(mediaTypeKey)
	mediaInfo := getMediaInfo(mediaType)

	buttonsInfo := ""
	if len(newButtons) > 0 {
		buttonsInfo = "\n🔘 Кнопки: " + strings.Join(newButtons, ", ")
	}

	_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    update.CallbackQuery.Message.Message.Chat.ID,
		MessageID: update.CallbackQuery.Message.Message.ID,
		Text: fmt.Sprintf(
			"🔘 <b>Выберите кнопки для рассылки</b>\n\n"+
				"Целевая аудитория: %s%s%s\n\n"+
				"<b>Текст:</b>\n%s\n\n"+
				"Нажмите на кнопки которые хотите добавить, затем \"Готово\".",
			targetName,
			mediaInfo,
			buttonsInfo,
			messageText,
		),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
}

func (h Handler) buildBroadcastButtonsKeyboard(selected []string) *models.InlineKeyboardMarkup {
	isSelected := func(name string) bool {
		for _, s := range selected {
			if s == name {
				return true
			}
		}
		return false
	}

	promoText := "🎟 Промокод"
	if isSelected("promo") {
		promoText = "✅ " + promoText
	}

	subText := "🌐 Ваша подписка"
	if isSelected("subscription") {
		subText = "✅ " + subText
	}

	buyText := "🛒 Купить"
	if isSelected("buy") {
		buyText = "✅ " + buyText
	}

	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: promoText, CallbackData: "broadcast_btn_promo"},
				{Text: subText, CallbackData: "broadcast_btn_subscription"},
			},
			{
				{Text: buyText, CallbackData: "broadcast_btn_buy"},
			},
			{
				{Text: "✅ Без кнопок / Готово", CallbackData: "broadcast_btn_done"},
			},
			{
				{Text: "🔙 Назад", CallbackData: "admin_broadcast"},
			},
		},
	}
}

func (h Handler) showBroadcastConfirmation(ctx context.Context, b *bot.Bot, update *models.Update) {
	userID := update.CallbackQuery.From.ID

	targetKey := fmt.Sprintf("broadcast_target_%d", userID)
	targetType, found := h.cache.GetString(targetKey)
	if !found {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Ошибка: данные рассылки не найдены",
			ShowAlert:       true,
		})
		return
	}

	textKey := fmt.Sprintf("broadcast_text_%d", userID)
	messageText, _ := h.cache.GetString(textKey)

	// Создаем запись в истории рассылок
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	broadcastID, err := h.broadcastService.CreateBroadcast(ctxWithTimeout, targetType, messageText)
	if err != nil {
		slog.Error("Failed to create broadcast", "error", err)
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Ошибка создания рассылки",
			ShowAlert:       true,
		})
		return
	}

	// Сохраняем ID рассылки
	h.cache.SetString(fmt.Sprintf("broadcast_id_%d", userID), fmt.Sprintf("%d", broadcastID), 600)

	targetName := getTargetName(targetType)

	// Получаем количество получателей
	recipientsCount, err := h.broadcastService.GetTargetCustomersCount(ctx, targetType)
	if err != nil {
		slog.Error("Failed to get recipients count", "error", err)
		recipientsCount = 0
	}

	mediaTypeKey := fmt.Sprintf("broadcast_media_type_%d", userID)
	mediaType, _ := h.cache.GetString(mediaTypeKey)
	mediaInfo := getMediaInfo(mediaType)

	buttonsKey := fmt.Sprintf("broadcast_buttons_%d", userID)
	buttons, _ := h.cache.GetString(buttonsKey)
	buttonsInfo := ""
	if buttons != "" {
		buttonsInfo = "\n🔘 Кнопки: " + buttons
	}

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: fmt.Sprintf("✅ Отправить %d получателям", recipientsCount), CallbackData: fmt.Sprintf("broadcast_confirm_%d", broadcastID)},
			},
			{
				{Text: "❌ Отменить", CallbackData: "admin_broadcast"},
			},
		},
	}

	_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    update.CallbackQuery.Message.Message.Chat.ID,
		MessageID: update.CallbackQuery.Message.Message.ID,
		Text: fmt.Sprintf(
			"📋 <b>Подтверждение рассылки</b>\n\n"+
				"Целевая аудитория: %s\n"+
				"👥 <b>Получателей: %d</b>%s%s\n\n"+
				"<b>Текст сообщения:</b>\n%s\n\n"+
				"Подтвердите отправку рассылки.",
			targetName,
			recipientsCount,
			mediaInfo,
			buttonsInfo,
			messageText,
		),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
}

func (h Handler) AdminBroadcastConfirmCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Доступ запрещён",
			ShowAlert:       true,
		})
		return
	}

	userID := update.CallbackQuery.From.ID

	broadcastIDStr := strings.TrimPrefix(update.CallbackQuery.Data, "broadcast_confirm_")
	broadcastID, err := strconv.ParseInt(broadcastIDStr, 10, 64)
	if err != nil {
		slog.Error("Invalid broadcast ID", "error", err)
		return
	}

	// Получаем информацию о рассылке
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	broadcastData, err := h.broadcastService.GetBroadcast(ctxWithTimeout, broadcastID)
	if err != nil {
		slog.Error("Failed to get broadcast", "error", err)
		return
	}

	// Получаем опции из кеша
	mediaKey := fmt.Sprintf("broadcast_media_%d", userID)
	mediaFileID, _ := h.cache.GetString(mediaKey)

	mediaTypeKey := fmt.Sprintf("broadcast_media_type_%d", userID)
	mediaType, _ := h.cache.GetString(mediaTypeKey)

	buttonsKey := fmt.Sprintf("broadcast_buttons_%d", userID)
	buttonsStr, _ := h.cache.GetString(buttonsKey)
	var buttons []string
	if buttonsStr != "" {
		for _, btn := range strings.Split(buttonsStr, ",") {
			if btn != "" {
				buttons = append(buttons, btn)
			}
		}
	}

	// Запускаем рассылку с опциями
	opts := &broadcast.BroadcastOptions{
		MediaType:   mediaType,
		MediaFileID: mediaFileID,
		Buttons:     buttons,
		MiniAppURL:  config.GetMiniAppURL(),
	}
	h.broadcastService.StartBroadcastWithOptions(ctx, broadcastID, broadcastData.TargetType, broadcastData.MessageText, opts)

	// Очищаем кеш
	h.cache.Delete(fmt.Sprintf("broadcast_target_%d", userID))
	h.cache.Delete(fmt.Sprintf("broadcast_text_%d", userID))
	h.cache.Delete(fmt.Sprintf("broadcast_media_%d", userID))
	h.cache.Delete(fmt.Sprintf("broadcast_media_type_%d", userID))
	h.cache.Delete(fmt.Sprintf("broadcast_buttons_%d", userID))
	h.cache.Delete(fmt.Sprintf("broadcast_id_%d", userID))
	h.cache.Delete(fmt.Sprintf("broadcast_state_%d", userID))

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    update.CallbackQuery.Message.Message.Chat.ID,
		MessageID: update.CallbackQuery.Message.Message.ID,
		Text:      "✅ <b>Рассылка запущена!</b>\n\nПрогресс можно отслеживать в разделе \"История рассылок\".",
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "📋 История рассылок", CallbackData: "admin_broadcast_history"}},
				{{Text: "🔙 В меню", CallbackData: "admin_broadcast"}},
			},
		},
	})

	if err != nil {
		slog.Error("Error editing message", "error", err)
	}

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		Text:            "Рассылка запущена!",
	})
}

func (h Handler) AdminBroadcastHistoryCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Доступ запрещён",
			ShowAlert:       true,
		})
		return
	}

	// Получаем историю с таймаутом
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	history, err := h.broadcastService.GetBroadcastHistory(ctxWithTimeout, 10, 0)
	if err != nil {
		slog.Error("Failed to get broadcast history", "error", err)
		return
	}

	text := "📊 <b>История рассылок</b>\n\nНажмите на рассылку для просмотра деталей:"

	var rows [][]models.InlineKeyboardButton

	if len(history) == 0 {
		text = "📊 <b>История рассылок</b>\n\nИстория пуста"
	} else {
		for _, item := range history {
			status := getStatusEmoji(item.Status)
			targetShort := getTargetShortName(item.TargetType)
			// Кнопка: статус дата | аудитория | sent/total
			btnText := fmt.Sprintf("%s %s | %s | %d/%d",
				status,
				item.CreatedAt.Format("02.01 15:04"),
				targetShort,
				item.SentCount,
				item.TotalCount,
			)
			rows = append(rows, []models.InlineKeyboardButton{
				{Text: btnText, CallbackData: fmt.Sprintf("broadcast_view_%d", item.ID)},
			})
		}
	}

	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "🔙 Назад", CallbackData: "admin_back"},
	})

	keyboard := &models.InlineKeyboardMarkup{InlineKeyboard: rows}

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      update.CallbackQuery.Message.Message.Chat.ID,
		MessageID:   update.CallbackQuery.Message.Message.ID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})

	if err != nil {
		slog.Error("Error editing message", "error", err)
	}

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
}

// AdminBroadcastViewCallback показывает детали рассылки
func (h Handler) AdminBroadcastViewCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Доступ запрещён",
			ShowAlert:       true,
		})
		return
	}

	broadcastIDStr := strings.TrimPrefix(update.CallbackQuery.Data, "broadcast_view_")
	broadcastID, err := strconv.ParseInt(broadcastIDStr, 10, 64)
	if err != nil {
		slog.Error("Invalid broadcast ID", "error", err)
		return
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	item, err := h.broadcastService.GetBroadcast(ctxWithTimeout, broadcastID)
	if err != nil {
		slog.Error("Failed to get broadcast", "error", err)
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Рассылка не найдена",
			ShowAlert:       true,
		})
		return
	}

	status := getStatusEmoji(item.Status)
	completedAt := "-"
	if item.CompletedAt != nil {
		completedAt = item.CompletedAt.Format("02.01.2006 15:04")
	}

	// Sanitize and truncate text
	msgPreview := strings.ToValidUTF8(item.MessageText, "")
	msgPreview = escapeHTML(msgPreview)
	runes := []rune(msgPreview)
	if len(runes) > 200 {
		msgPreview = string(runes[:200]) + "..."
	}

	text := fmt.Sprintf(
		"<b>Рассылка #%d</b>\n\n"+
			"%s Статус: %s\n"+
			"Аудитория: %s\n"+
			"Отправлено: %d/%d\n"+
			"Ошибок: %d\n"+
			"Создана: %s\n"+
			"Завершена: %s\n\n"+
			"<b>Текст:</b>\n%s",
		item.ID,
		status,
		item.Status,
		getTargetName(item.TargetType),
		item.SentCount,
		item.TotalCount,
		item.FailedCount,
		item.CreatedAt.Format("02.01.2006 15:04"),
		completedAt,
		msgPreview,
	)

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "Удалить", CallbackData: fmt.Sprintf("broadcast_delete_%d", item.ID)},
			},
			{
				{Text: "Назад", CallbackData: "admin_broadcast_history"},
			},
		},
	}

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      update.CallbackQuery.Message.Message.Chat.ID,
		MessageID:   update.CallbackQuery.Message.Message.ID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})

	if err != nil {
		slog.Error("Error editing message", "error", err)
	}

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
}

// AdminBroadcastDeleteCallback удаляет рассылку из истории
func (h Handler) AdminBroadcastDeleteCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Доступ запрещён",
			ShowAlert:       true,
		})
		return
	}

	broadcastIDStr := strings.TrimPrefix(update.CallbackQuery.Data, "broadcast_delete_")
	broadcastID, err := strconv.ParseInt(broadcastIDStr, 10, 64)
	if err != nil {
		slog.Error("Invalid broadcast ID", "error", err)
		return
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err = h.broadcastService.DeleteBroadcast(ctxWithTimeout, broadcastID)
	if err != nil {
		slog.Error("Failed to delete broadcast", "error", err)
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Ошибка удаления",
			ShowAlert:       true,
		})
		return
	}

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		Text:            "✅ Рассылка удалена",
	})

	// Возвращаемся к списку
	h.AdminBroadcastHistoryCallback(ctx, b, update)
}

func (h Handler) AdminBackCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	// Сразу отвечаем на callback чтобы убрать "часики"
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	// Очищаем все состояния админа
	userID := update.CallbackQuery.From.ID
	h.cache.Delete(fmt.Sprintf("broadcast_state_%d", userID))
	h.cache.Delete(fmt.Sprintf("broadcast_target_%d", userID))
	h.cache.Delete(fmt.Sprintf("admin_promo_state_%d", userID))
	h.cache.Delete(fmt.Sprintf("promo_state_%d", userID))

	// Удаляем старое сообщение
	_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    update.CallbackQuery.Message.Message.Chat.ID,
		MessageID: update.CallbackQuery.Message.Message.ID,
	})

	// Отправляем новое
	h.AdminCommandHandler(ctx, b, &models.Update{
		Message: &models.Message{
			From: &update.CallbackQuery.From,
			Chat: models.Chat{ID: update.CallbackQuery.Message.Message.Chat.ID},
		},
	})
}

func (h Handler) AdminCloseCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	// Сразу отвечаем на callback
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	// Очищаем все состояния админа
	userID := update.CallbackQuery.From.ID
	h.cache.Delete(fmt.Sprintf("broadcast_state_%d", userID))
	h.cache.Delete(fmt.Sprintf("broadcast_target_%d", userID))
	h.cache.Delete(fmt.Sprintf("admin_promo_state_%d", userID))
	h.cache.Delete(fmt.Sprintf("promo_state_%d", userID))

	_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    update.CallbackQuery.Message.Message.Chat.ID,
		MessageID: update.CallbackQuery.Message.Message.ID,
	})
}

// AdminTextInputHandler - объединённый обработчик текстового ввода для админа
func (h Handler) AdminTextInputHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.From.ID != config.GetAdminTelegramId() {
		return
	}

	userID := update.Message.From.ID

	// Проверяем состояние создания промокода (админ)
	promoStateKey := fmt.Sprintf("admin_promo_state_%d", userID)
	if state, found := h.cache.GetString(promoStateKey); found && state == "waiting_code" {
		h.AdminPromoCreateInputHandler(ctx, b, update)
		return
	}

	// Проверяем состояние создания промокода на тариф (админ)
	promoTariffStateKey := fmt.Sprintf("admin_promo_tariff_state_%d", userID)
	if state, found := h.cache.GetString(promoTariffStateKey); found && state == "waiting_code" {
		h.AdminPromoTariffCreateInputHandler(ctx, b, update)
		return
	}

	// Проверяем состояние рассылки
	broadcastStateKey := fmt.Sprintf("broadcast_state_%d", userID)
	if state, found := h.cache.GetString(broadcastStateKey); found && state == "waiting_message" {
		h.AdminBroadcastMessageHandler(ctx, b, update)
		return
	}

	// Проверяем состояние ввода промокода (как пользователь)
	userPromoStateKey := fmt.Sprintf("promo_state_%d", userID)
	if state, found := h.cache.GetString(userPromoStateKey); found && state == "waiting_code" {
		h.PromoCodeInputHandler(ctx, b, update)
		return
	}
}

// Helper functions

func getTargetName(targetType string) string {
	switch targetType {
	case "all":
		return "Все пользователи"
	case "with_subscription":
		return "С подпиской"
	case "without_subscription":
		return "Без подписки"
	case "expiring":
		return "С истекающей подпиской (3 дня)"
	case "start_only":
		return "Только нажали /start"
	default:
		return "Неизвестно"
	}
}

func getStatusEmoji(status string) string {
	switch status {
	case "completed":
		return "✅"
	case "in_progress":
		return "⏳"
	case "partial":
		return "✅"
	case "failed":
		return "❌"
	case "pending":
		return "🕐"
	default:
		return "❓"
	}
}

func getTargetShortName(targetType string) string {
	switch targetType {
	case "all":
		return "Все"
	case "with_subscription":
		return "С подп."
	case "without_subscription":
		return "Без подп."
	case "expiring":
		return "Истекает"
	case "start_only":
		return "/start"
	default:
		return "?"
	}
}

// escapeHTML экранирует HTML символы для безопасного отображения в Telegram
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
