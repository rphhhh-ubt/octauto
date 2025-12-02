package handler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
)

// NotificationTester интерфейс для тестирования уведомлений
type NotificationTester interface {
	ProcessTrialInactiveNotifications() error
}

// notificationTester хранит ссылку на сервис уведомлений
var notificationTester NotificationTester

// SetNotificationTester устанавливает сервис для тестирования уведомлений
func SetNotificationTester(tester NotificationTester) {
	notificationTester = tester
}

// AdminTestNotificationsCallback показывает меню тестирования уведомлений
func (h Handler) AdminTestNotificationsCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Доступ запрещён",
			ShowAlert:       true,
		})
		return
	}

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "📵 Тест: Неактивный триал", CallbackData: "admin_test_inactive_trial"},
			},
			{
				{Text: "🔙 Назад", CallbackData: "admin_back"},
			},
		},
	}

	text := "🧪 <b>Тестирование уведомлений</b>\n\n" +
		"<b>Неактивный триал:</b>\n" +
		"Отправит уведомление триальным пользователям, которые:\n" +
		"• Создали аккаунт > 1 часа назад\n" +
		"• Ещё не подключались (firstConnectedAt = null)\n" +
		"• Не получали это уведомление ранее\n\n" +
		"<b>Winback:</b>\n" +
		"Теперь обрабатывается автоматически через вебхук Remnawave (user.expired_24_hours_ago)\n\n" +
		"⚠️ Это реальная отправка уведомлений!"

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
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

// AdminTestInactiveTrialCallback запускает тест уведомлений о неактивности триала
func (h Handler) AdminTestInactiveTrialCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery.From.ID != config.GetAdminTelegramId() {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Доступ запрещён",
			ShowAlert:       true,
		})
		return
	}

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		Text:            "Запускаю проверку...",
	})

	if notificationTester == nil {
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    update.CallbackQuery.Message.Message.Chat.ID,
			MessageID: update.CallbackQuery.Message.Message.ID,
			Text:      "❌ Сервис уведомлений не инициализирован",
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	if !config.IsTrialInactiveNotificationEnabled() {
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    update.CallbackQuery.Message.Message.Chat.ID,
			MessageID: update.CallbackQuery.Message.Message.ID,
			Text:      "❌ Уведомления о неактивности триала отключены\n\nВключите TRIAL_INACTIVE_NOTIFICATION_ENABLED=true в .env",
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	start := time.Now()
	err := notificationTester.ProcessTrialInactiveNotifications()
	duration := time.Since(start)

	var resultText string
	if err != nil {
		resultText = fmt.Sprintf("❌ Ошибка: %v\n\nВремя: %v", err, duration)
		slog.Error("Test inactive trial notifications failed", "error", err)
	} else {
		resultText = fmt.Sprintf("✅ Проверка завершена!\n\nВремя: %v\n\nПроверьте логи для деталей.", duration)
		slog.Info("Test inactive trial notifications completed", "duration", duration)
	}

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "🔙 Назад", CallbackData: "admin_test_notifications"},
			},
		},
	}

	_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      update.CallbackQuery.Message.Message.Chat.ID,
		MessageID:   update.CallbackQuery.Message.Message.ID,
		Text:        resultText,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
}

// AdminTestWinbackCallback - deprecated, winback теперь через вебхук
func (h Handler) AdminTestWinbackCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		Text:            "Winback теперь через вебхук Remnawave",
		ShowAlert:       true,
	})
}
