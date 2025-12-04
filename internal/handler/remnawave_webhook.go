package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/yookasa"
	"remnawave-tg-shop-bot/utils"
)

// WebhookPayload представляет payload от Remnawave webhook
type WebhookPayload struct {
	Event     string      `json:"event"`
	Data      WebhookUser `json:"data"`
	Timestamp string      `json:"timestamp"`
}

// WebhookUser представляет данные пользователя из webhook payload
type WebhookUser struct {
	UUID             string          `json:"uuid"`
	TelegramID       json.Number     `json:"telegramId"`
	FirstConnectedAt *time.Time      `json:"firstConnectedAt"`
	ExpireAt         time.Time       `json:"expireAt"`
	Status           string          `json:"status"`
}

// GetTelegramID возвращает telegramId как int64
func (u WebhookUser) GetTelegramID() *int64 {
	if u.TelegramID == "" {
		return nil
	}
	id, err := u.TelegramID.Int64()
	if err != nil {
		return nil
	}
	return &id
}

// customerRepository интерфейс для работы с клиентами
type customerRepository interface {
	FindByTelegramId(ctx context.Context, telegramId int64) (*database.Customer, error)
	UpdateWinbackOffer(ctx context.Context, id int64, sentAt, expiresAt time.Time, price, devices, months int) error
	UpdateRecurringNotifiedAt(ctx context.Context, id int64, notifiedAt time.Time) error
	DisableRecurring(ctx context.Context, id int64) error
}

// purchaseRepository интерфейс для проверки оплаченных покупок
type purchaseRepository interface {
	HasPaidPurchases(ctx context.Context, customerID int64) (bool, error)
	HasRecentPaidPurchase(ctx context.Context, customerID int64, withinMinutes int) (bool, error)
}

// yookasaClient интерфейс для работы с YooKassa API
type yookasaClient interface {
	CreateRecurringPayment(ctx context.Context, paymentMethodID uuid.UUID, amount int, months int, customerId int64, description string) (*yookasa.Payment, error)
}

// remnawaveClient интерфейс для работы с Remnawave API
type remnawaveClient interface {
	CreateOrUpdateUserWithDeviceLimit(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, days int, isTrialUser bool, deviceLimit *int) (*remapi.UserResponseResponse, error)
}

// translationManager интерфейс для работы с переводами
type translationManager interface {
	GetText(langCode, key string) string
}

// telegramBotClient интерфейс для работы с Telegram Bot API
type telegramBotClient interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
}

// RemnawaveWebhookHandler обрабатывает webhooks от Remnawave
type RemnawaveWebhookHandler struct {
	tm             translationManager
	telegramBot    telegramBotClient
	customerRepo   customerRepository
	purchaseRepo   purchaseRepository
	webhookSecret  string
	yookasa        yookasaClient
	remnawave      remnawaveClient
}

// NewRemnawaveWebhookHandler создаёт новый handler для Remnawave webhooks
func NewRemnawaveWebhookHandler(
	tm translationManager,
	telegramBot telegramBotClient,
	customerRepo customerRepository,
	purchaseRepo purchaseRepository,
) *RemnawaveWebhookHandler {
	return &RemnawaveWebhookHandler{
		tm:            tm,
		telegramBot:   telegramBot,
		customerRepo:  customerRepo,
		purchaseRepo:  purchaseRepo,
		webhookSecret: config.GetRemnawaveWebhookSecret(),
	}
}

// SetYookasaClient устанавливает YooKassa клиент для рекуррентных платежей
func (h *RemnawaveWebhookHandler) SetYookasaClient(client yookasaClient) {
	h.yookasa = client
}

// SetRemnawaveClient устанавливает Remnawave клиент для продления подписки
func (h *RemnawaveWebhookHandler) SetRemnawaveClient(client remnawaveClient) {
	h.remnawave = client
}


// validateSignature проверяет подпись webhook запроса
// Возвращает true если HMAC-SHA256(body, secret) == X-Remnawave-Signature
func (h *RemnawaveWebhookHandler) validateSignature(body []byte, signature string) bool {
	if h.webhookSecret == "" {
		slog.Warn("Remnawave webhook secret not configured, skipping signature validation")
		return true
	}

	mac := hmac.New(sha256.New, []byte(h.webhookSecret))
	mac.Write(body)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expectedSignature), []byte(signature))
}

// HandleWebhook обрабатывает входящий webhook от Remnawave
func (h *RemnawaveWebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Failed to read webhook body", "error", err)
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Проверяем подпись
	signature := r.Header.Get("X-Remnawave-Signature")
	if !h.validateSignature(body, signature) {
		slog.Warn("Invalid webhook signature")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Парсим payload
	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		slog.Error("Failed to parse webhook payload", "error", err)
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	// Роутим по типу события (логируем только обработанные события)
	ctx := r.Context()
	switch payload.Event {
	case "user.expires_in_48_hours":
		if err := h.processUserExpiresIn48Hours(ctx, payload.Data); err != nil {
			slog.Error("Failed to process user.expires_in_48_hours", "error", err)
		}
	case "user.expires_in_24_hours":
		if err := h.processUserExpiresIn24Hours(ctx, payload.Data); err != nil {
			slog.Error("Failed to process user.expires_in_24_hours", "error", err)
		}
	case "user.expired":
		if err := h.processUserExpired(ctx, payload.Data); err != nil {
			slog.Error("Failed to process user.expired", "error", err)
		}
	case "user.expired_24_hours_ago":
		if err := h.processUserExpired24HoursAgo(ctx, payload.Data); err != nil {
			slog.Error("Failed to process user.expired_24_hours_ago", "error", err)
		}
	default:
		// Игнорируем неизвестные события без логирования
	}

	// Всегда возвращаем 200 OK чтобы Remnawave не ретраил
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// processUserExpiresIn48Hours обрабатывает событие истечения через 48 часов
// Сейчас не используется для уведомлений (перенесено на 24 часа)
func (h *RemnawaveWebhookHandler) processUserExpiresIn48Hours(ctx context.Context, user WebhookUser) error {
	// Уведомление о предстоящем списании теперь отправляется за 24 часа
	// См. processUserExpiresIn24Hours
	return nil
}

// processUserExpiresIn24Hours обрабатывает событие истечения через 24 часа
// Для пользователей с автопродлением — уведомление о предстоящем списании
// Для остальных — уведомление об истечении подписки
func (h *RemnawaveWebhookHandler) processUserExpiresIn24Hours(ctx context.Context, user WebhookUser) error {
	// Проверяем firstConnectedAt
	if user.FirstConnectedAt == nil {
		slog.Debug("Skipping notification for user without firstConnectedAt", "uuid", user.UUID)
		return nil
	}

	telegramID := user.GetTelegramID()
	if telegramID == nil {
		slog.Warn("User has no telegramId", "uuid", user.UUID)
		return nil
	}

	// Получаем customer из БД
	customer, err := h.customerRepo.FindByTelegramId(ctx, *telegramID)
	if err != nil {
		return fmt.Errorf("failed to find customer: %w", err)
	}

	lang := config.DefaultLanguage()
	if customer != nil && customer.Language != "" {
		lang = customer.Language
	}

	// Проверяем автопродление
	if config.IsRecurringPaymentsEnabled() && customer != nil && customer.RecurringEnabled && customer.PaymentMethodID != nil {
		// Формируем сумму списания
		amount := 0
		if customer.RecurringAmount != nil {
			amount = *customer.RecurringAmount
		}

		// Уведомление о предстоящем списании
		message := fmt.Sprintf(
			h.tm.GetText(lang, "recurring_charge_notification"),
			amount,
		)

		// Кнопка управления сохранёнными способами оплаты
		keyboard := &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{Text: h.tm.GetText(lang, "saved_payment_methods_button"), CallbackData: CallbackSavedPaymentMethods + "?from=notification"},
				},
			},
		}

		_, err = h.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      *telegramID,
			Text:        message,
			ParseMode:   "HTML",
			ReplyMarkup: keyboard,
		})
		if err != nil {
			return fmt.Errorf("failed to send recurring notification: %w", err)
		}

		slog.Info("Sent recurring charge notification (24h)", "telegramId", utils.MaskHalfInt64(*telegramID), "amount", amount)
		return nil
	}

	// Обычное уведомление об истечении подписки
	message := h.tm.GetText(lang, "subscription_expiring_1day")

	// Кнопка продления
	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: h.tm.GetText(lang, "renew_subscription_button"), CallbackData: CallbackBuy},
			},
		},
	}

	_, err = h.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      *telegramID,
		Text:        message,
		ParseMode:   "HTML",
		ReplyMarkup: keyboard,
	})
	if err != nil {
		return fmt.Errorf("failed to send telegram message: %w", err)
	}

	slog.Info("Sent 24-hour expiration notification", "telegramId", utils.MaskHalfInt64(*telegramID))
	return nil
}

// processUserExpired обрабатывает событие истечения подписки
// Если у пользователя включено автопродление - выполняет автоплатёж
func (h *RemnawaveWebhookHandler) processUserExpired(ctx context.Context, user WebhookUser) error {
	// Проверяем firstConnectedAt
	if user.FirstConnectedAt == nil {
		slog.Debug("Skipping notification for user without firstConnectedAt", "uuid", user.UUID)
		return nil
	}

	telegramID := user.GetTelegramID()
	if telegramID == nil {
		slog.Warn("User has no telegramId", "uuid", user.UUID)
		return nil
	}

	// Получаем customer из БД
	customer, err := h.customerRepo.FindByTelegramId(ctx, *telegramID)
	if err != nil {
		return fmt.Errorf("failed to find customer: %w", err)
	}

	lang := config.DefaultLanguage()
	if customer != nil && customer.Language != "" {
		lang = customer.Language
	}

	// Проверяем автопродление
	if config.IsRecurringPaymentsEnabled() && customer != nil && customer.RecurringEnabled && customer.PaymentMethodID != nil {
		// Пытаемся выполнить автоплатёж
		err := h.processRecurringPayment(ctx, customer, *telegramID, lang)
		if err != nil {
			slog.Error("Recurring payment failed", "telegramId", utils.MaskHalfInt64(*telegramID), "error", err)
			// При ошибке отправляем уведомление о неудачном списании
			h.sendRecurringFailedNotification(ctx, *telegramID, lang)
		}
		return nil
	}

	// Стандартное уведомление об истечении подписки
	message := h.tm.GetText(lang, "subscription_expired")

	// Кнопка продления
	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: h.tm.GetText(lang, "renew_subscription_button"), CallbackData: CallbackBuy},
			},
		},
	}

	// Отправляем уведомление с кнопкой
	_, err = h.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      *telegramID,
		Text:        message,
		ParseMode:   "HTML",
		ReplyMarkup: keyboard,
	})
	if err != nil {
		return fmt.Errorf("failed to send telegram message: %w", err)
	}

	slog.Info("Sent expired notification", "telegramId", utils.MaskHalfInt64(*telegramID))
	return nil
}

// processRecurringPayment выполняет автоматическое списание для пользователя с автопродлением
func (h *RemnawaveWebhookHandler) processRecurringPayment(ctx context.Context, customer *database.Customer, telegramID int64, lang string) error {
	if h.yookasa == nil || h.remnawave == nil {
		return fmt.Errorf("yookasa or remnawave client not configured")
	}

	// Защита от race condition: проверяем что не было платежа за последнюю минуту
	// Это предотвращает двойное списание если webhook придёт дважды
	if h.purchaseRepo != nil {
		hasRecent, err := h.purchaseRepo.HasRecentPaidPurchase(ctx, customer.ID, 1)
		if err != nil {
			slog.Warn("Failed to check recent purchases, proceeding with caution", "error", err)
		} else if hasRecent {
			slog.Info("Skipping recurring payment - recent payment exists", "customerId", utils.MaskHalfInt64(customer.ID))
			return nil
		}
	}

	// Парсим payment_method_id
	paymentMethodID, err := uuid.Parse(*customer.PaymentMethodID)
	if err != nil {
		return fmt.Errorf("invalid payment_method_id: %w", err)
	}

	// Получаем параметры автопродления
	amount := 0
	if customer.RecurringAmount != nil {
		amount = *customer.RecurringAmount
	}
	if amount == 0 {
		return fmt.Errorf("recurring amount is zero")
	}

	months := 1
	if customer.RecurringMonths != nil {
		months = *customer.RecurringMonths
	}

	// Формируем описание платежа
	var monthString string
	switch months {
	case 1:
		monthString = "месяц"
	case 3, 4:
		monthString = "месяца"
	default:
		monthString = "месяцев"
	}
	description := fmt.Sprintf("Автопродление подписки на %d %s", months, monthString)

	// Создаём автоплатёж
	payment, err := h.yookasa.CreateRecurringPayment(ctx, paymentMethodID, amount, months, customer.ID, description)
	if err != nil {
		return fmt.Errorf("failed to create recurring payment: %w", err)
	}

	// Проверяем результат платежа
	if payment.IsCancelled() {
		// Проверяем причину отмены
		if payment.IsPermissionRevoked() {
			// Отзыв разрешения - отключаем автопродление
			if err := h.customerRepo.DisableRecurring(ctx, customer.ID); err != nil {
				slog.Error("Failed to disable recurring after permission_revoked", "customerId", utils.MaskHalfInt64(customer.ID), "error", err)
			}
			h.sendPermissionRevokedNotification(ctx, telegramID, lang)
			slog.Info("Recurring disabled due to permission_revoked", "telegramId", utils.MaskHalfInt64(telegramID))
			return nil
		}
		return fmt.Errorf("payment cancelled: %s", payment.CancellationDetails.Reason)
	}

	if !payment.IsSucceeded() {
		return fmt.Errorf("payment not succeeded, status: %s", payment.Status)
	}

	// Платёж успешен - продлеваем подписку
	days := months * config.DaysInMonth()

	// Получаем лимит устройств из тарифа если есть
	var deviceLimit *int
	if customer.RecurringTariffName != nil {
		tariff := config.GetTariffByName(*customer.RecurringTariffName)
		if tariff != nil {
			deviceLimit = &tariff.Devices
		}
	}

	_, err = h.remnawave.CreateOrUpdateUserWithDeviceLimit(ctx, customer.ID, telegramID, config.TrafficLimit(), days, false, deviceLimit)
	if err != nil {
		slog.Error("Failed to extend subscription after recurring payment", "telegramId", utils.MaskHalfInt64(telegramID), "error", err)
		return fmt.Errorf("failed to extend subscription: %w", err)
	}

	// Отправляем уведомление об успешном продлении
	h.sendRecurringSuccessNotification(ctx, telegramID, lang, amount, months)

	slog.Info("Recurring payment successful", "telegramId", utils.MaskHalfInt64(telegramID), "amount", amount, "months", months)
	return nil
}

// sendRecurringSuccessNotification отправляет уведомление об успешном автопродлении
func (h *RemnawaveWebhookHandler) sendRecurringSuccessNotification(ctx context.Context, telegramID int64, lang string, amount int, months int) {
	message := h.tm.GetText(lang, "recurring_success_simple")

	_, err := h.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    telegramID,
		Text:      message,
		ParseMode: "HTML",
	})
	if err != nil {
		slog.Error("Failed to send recurring success notification", "telegramId", utils.MaskHalfInt64(telegramID), "error", err)
	}
}

// sendRecurringFailedNotification отправляет уведомление о неудачном автоплатеже
func (h *RemnawaveWebhookHandler) sendRecurringFailedNotification(ctx context.Context, telegramID int64, lang string) {
	message := h.tm.GetText(lang, "recurring_failed")

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: h.tm.GetText(lang, "renew_subscription_button"), CallbackData: CallbackBuy},
			},
		},
	}

	_, err := h.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      telegramID,
		Text:        message,
		ParseMode:   "HTML",
		ReplyMarkup: keyboard,
	})
	if err != nil {
		slog.Error("Failed to send recurring failed notification", "telegramId", utils.MaskHalfInt64(telegramID), "error", err)
	}
}

// sendPermissionRevokedNotification отправляет уведомление об отзыве разрешения на автоплатежи
func (h *RemnawaveWebhookHandler) sendPermissionRevokedNotification(ctx context.Context, telegramID int64, lang string) {
	message := h.tm.GetText(lang, "recurring_permission_revoked")

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: h.tm.GetText(lang, "renew_subscription_button"), CallbackData: CallbackBuy},
			},
		},
	}

	_, err := h.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      telegramID,
		Text:        message,
		ParseMode:   "HTML",
		ReplyMarkup: keyboard,
	})
	if err != nil {
		slog.Error("Failed to send permission revoked notification", "telegramId", utils.MaskHalfInt64(telegramID), "error", err)
	}
}

// processUserExpired24HoursAgo обрабатывает событие истечения подписки 24 часа назад (winback)
func (h *RemnawaveWebhookHandler) processUserExpired24HoursAgo(ctx context.Context, user WebhookUser) error {
	if !config.IsWinbackEnabled() {
		slog.Debug("Winback disabled, skipping", "uuid", user.UUID)
		return nil
	}

	telegramID := user.GetTelegramID()
	if telegramID == nil {
		slog.Warn("User has no telegramId for winback", "uuid", user.UUID)
		return nil
	}

	// Получаем customer из БД
	customer, err := h.customerRepo.FindByTelegramId(ctx, *telegramID)
	if err != nil {
		return fmt.Errorf("failed to find customer: %w", err)
	}
	if customer == nil {
		slog.Warn("Customer not found for winback", "telegramId", utils.MaskHalfInt64(*telegramID))
		return nil
	}

	// Проверяем что winback ещё не отправлялся
	if customer.WinbackOfferSentAt != nil {
		slog.Debug("Winback already sent", "customerId", utils.MaskHalfInt64(customer.ID))
		return nil
	}

	// Проверяем что у пользователя НЕТ оплаченных покупок (только триальные)
	hasPaid, err := h.purchaseRepo.HasPaidPurchases(ctx, customer.ID)
	if err != nil {
		return fmt.Errorf("failed to check paid purchases: %w", err)
	}
	if hasPaid {
		slog.Debug("User has paid purchases, skipping winback", "customerId", utils.MaskHalfInt64(customer.ID))
		return nil
	}

	// Получаем параметры winback из конфига
	now := time.Now()
	price := config.GetWinbackPrice()
	devices := config.GetWinbackDevices()
	months := config.GetWinbackMonths()
	validHours := config.GetWinbackValidHours()
	expiresAt := now.Add(time.Duration(validHours) * time.Hour)

	lang := config.DefaultLanguage()
	if customer.Language != "" {
		lang = customer.Language
	}

	// Формируем сообщение winback
	message := fmt.Sprintf(
		h.tm.GetText(lang, "winback_offer"),
		price,
		devices,
		expiresAt.Format("02.01.2006 15:04"),
	)

	// Кнопка активации winback
	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: h.tm.GetText(lang, "winback_activate_button"), CallbackData: CallbackWinbackActivate},
			},
		},
	}

	// Отправляем уведомление
	_, err = h.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      *telegramID,
		Text:        message,
		ParseMode:   "HTML",
		ReplyMarkup: keyboard,
	})
	if err != nil {
		return fmt.Errorf("failed to send winback message: %w", err)
	}

	// Сохраняем информацию о предложении в БД
	err = h.customerRepo.UpdateWinbackOffer(ctx, customer.ID, now, expiresAt, price, devices, months)
	if err != nil {
		return fmt.Errorf("failed to update winback offer: %w", err)
	}

	slog.Info("Sent winback offer via webhook",
		"customerId", utils.MaskHalfInt64(customer.ID),
		"price", price,
		"devices", devices,
		"months", months)
	return nil
}
