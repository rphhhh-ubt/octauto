package notification

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/handler"
	"remnawave-tg-shop-bot/internal/translation"
)

type customerRepository interface {
	FindByExpirationRange(ctx context.Context, startDate, endDate time.Time) (*[]database.Customer, error)
	FindTrialUsersForInactiveNotification(ctx context.Context) ([]database.Customer, error)
	UpdateTrialInactiveNotifiedAt(ctx context.Context, id int64, notifiedAt time.Time) error
}

type remnawaveClient interface {
	GetUserByTelegramID(ctx context.Context, telegramID int64) (*RemnawaveUserInfo, error)
}

// RemnawaveUserInfo содержит информацию о пользователе из Remnawave API
type RemnawaveUserInfo struct {
	UUID             uuid.UUID
	Username         string
	FirstConnectedAt *time.Time
	ExpireAt         time.Time
	Status           string
}

type tributeRepository interface {
	FindLatestActiveTributesByCustomerIDs(ctx context.Context, customerIDs []int64) (*[]database.Purchase, error)
}

type paymentProcessor interface {
	CreatePurchase(ctx context.Context, amount float64, months int, customer *database.Customer, invoiceType database.InvoiceType) (string, int64, error)
	ProcessPurchaseById(ctx context.Context, purchaseId int64) error
}

type SubscriptionService struct {
	customerRepository customerRepository
	purchaseRepository tributeRepository
	paymentService     paymentProcessor
	telegramBot        *bot.Bot
	tm                 *translation.Manager
	remnawaveClient    remnawaveClient
}

func NewSubscriptionService(customerRepository customerRepository,
	purchaseRepository tributeRepository,
	paymentService paymentProcessor,
	telegramBot *bot.Bot,
	tm *translation.Manager) *SubscriptionService {
	return &SubscriptionService{customerRepository: customerRepository, purchaseRepository: purchaseRepository, paymentService: paymentService, telegramBot: telegramBot, tm: tm}
}

// SetRemnawaveClient устанавливает клиент Remnawave для проверки firstConnectedAt
func (s *SubscriptionService) SetRemnawaveClient(client remnawaveClient) {
	s.remnawaveClient = client
}

// shouldSendInactiveNotification проверяет, нужно ли отправить уведомление о неактивности триала
// Условия: триал начался >= 1 час назад, firstConnectedAt == nil, уведомление ещё не отправлялось
// **Feature: trial-notifications, Property 2: Inactive Notification Eligibility**
// **Validates: Requirements 2.1, 2.3, 2.4**
func ShouldSendInactiveNotification(customer *database.Customer, firstConnectedAt *time.Time, now time.Time) bool {
	// Проверяем что уведомление ещё не отправлялось
	if customer.TrialInactiveNotifiedAt != nil {
		return false
	}

	// Проверяем что пользователь ещё не подключался
	if firstConnectedAt != nil {
		return false
	}

	// Проверяем что триал начался >= 1 час назад
	oneHourAgo := now.Add(-1 * time.Hour)
	if customer.CreatedAt.After(oneHourAgo) {
		return false
	}

	return true
}

// ProcessTrialInactiveNotifications обрабатывает отправку уведомлений неактивным триальным пользователям
// Получает список триальных пользователей, проверяет firstConnectedAt через Remnawave API, отправляет уведомления
// **Validates: Requirements 2.1, 2.2**
func (s *SubscriptionService) ProcessTrialInactiveNotifications() error {
	if !config.IsTrialInactiveNotificationEnabled() {
		return nil
	}

	if s.remnawaveClient == nil {
		slog.Warn("Remnawave client not set, skipping trial inactive notifications")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Получаем триальных пользователей для проверки
	customers, err := s.customerRepository.FindTrialUsersForInactiveNotification(ctx)
	if err != nil {
		slog.Error("Failed to find trial users for inactive notification", "error", err)
		return err
	}

	if len(customers) == 0 {
		return nil
	}

	slog.Info("Found trial users for inactive notification check", "count", len(customers))

	now := time.Now()
	notificationsSent := 0

	for _, customer := range customers {
		// Получаем информацию о пользователе из Remnawave по telegram_id
		userInfo, err := s.remnawaveClient.GetUserByTelegramID(ctx, customer.TelegramID)
		if err != nil {
			slog.Warn("Failed to get user info from Remnawave", "customer_id", customer.ID, "error", err)
			continue
		}

		// Проверяем условия отправки
		if !ShouldSendInactiveNotification(&customer, userInfo.FirstConnectedAt, now) {
			continue
		}

		// Отправляем уведомление
		err = s.sendInactiveTrialNotification(ctx, customer)
		if err != nil {
			slog.Error("Failed to send inactive trial notification", "customer_id", customer.ID, "error", err)
			continue
		}

		// Обновляем время отправки уведомления
		err = s.customerRepository.UpdateTrialInactiveNotifiedAt(ctx, customer.ID, now)
		if err != nil {
			slog.Error("Failed to update trial inactive notified at", "customer_id", customer.ID, "error", err)
			continue
		}

		notificationsSent++
		slog.Info("Sent inactive trial notification", "customer_id", customer.ID)
	}

	slog.Info("Processed trial inactive notifications", "sent", notificationsSent, "total_checked", len(customers))
	return nil
}

// sendInactiveTrialNotification отправляет уведомление о неактивности триала
// Включает кнопку "📱 Ваша подписка" с ссылкой на мини-апп
// **Feature: trial-notifications, Property 5: Inactive Notification Message Contains MiniApp Button**
// **Validates: Requirements 2.2**
func (s *SubscriptionService) sendInactiveTrialNotification(ctx context.Context, customer database.Customer) error {
	messageText := s.tm.GetText(customer.Language, "trial_inactive_notification")

	keyboard := BuildInactiveNotificationKeyboard(customer.Language, s.tm)

	_, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    customer.TelegramID,
		Text:      messageText,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
	})

	return err
}

// BuildInactiveNotificationKeyboard создаёт клавиатуру для уведомления о неактивности
// Содержит кнопку с ссылкой на мини-апп
// **Feature: trial-notifications, Property 5: Inactive Notification Message Contains MiniApp Button**
func BuildInactiveNotificationKeyboard(language string, tm *translation.Manager) [][]models.InlineKeyboardButton {
	miniAppURL := config.GetMiniAppURL()
	return BuildInactiveNotificationKeyboardWithURL(language, tm, miniAppURL)
}

// BuildInactiveNotificationKeyboardWithURL создаёт клавиатуру для уведомления о неактивности с указанным URL
// Эта функция используется для тестирования
// **Feature: trial-notifications, Property 5: Inactive Notification Message Contains MiniApp Button**
func BuildInactiveNotificationKeyboardWithURL(language string, tm *translation.Manager, miniAppURL string) [][]models.InlineKeyboardButton {
	var keyboard [][]models.InlineKeyboardButton

	// Хелпер для получения текста (обрабатывает nil tm)
	getText := func(key string) string {
		if tm != nil {
			return tm.GetText(language, key)
		}
		return key
	}

	if miniAppURL != "" {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{
				Text: getText("your_subscription_button"),
				WebApp: &models.WebAppInfo{
					URL: miniAppURL,
				},
			},
		})
	} else {
		// Fallback на callback если мини-апп не настроен
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{
				Text:         getText("connect_button"),
				CallbackData: handler.CallbackConnect,
			},
		})
	}

	return keyboard
}

// ShouldSendWinbackOffer проверяет, нужно ли отправить winback предложение
// Условия: триал истёк >= 24 часа назад, предложение ещё не отправлялось
// **Feature: trial-notifications, Property 3: Winback Offer Eligibility**
// **Validates: Requirements 3.1, 3.3**
func ShouldSendWinbackOffer(customer *database.Customer, now time.Time) bool {
	// Проверяем что предложение ещё не отправлялось
	if customer.WinbackOfferSentAt != nil {
		return false
	}

	// Проверяем что есть дата истечения
	if customer.ExpireAt == nil {
		return false
	}

	// Проверяем что триал истёк >= 24 часа назад
	twentyFourHoursAgo := now.Add(-24 * time.Hour)
	if customer.ExpireAt.After(twentyFourHoursAgo) {
		return false
	}

	return true
}

// Winback теперь обрабатывается через вебхук user.expired_24_hours_ago от Remnawave
// См. internal/handler/remnawave_webhook.go

// CallbackWinbackActivate - callback для активации winback предложения
const CallbackWinbackActivate = "winback_activate"

// BuildWinbackOfferKeyboard создаёт клавиатуру для winback предложения
func BuildWinbackOfferKeyboard(language string, tm *translation.Manager) [][]models.InlineKeyboardButton {
	var keyboard [][]models.InlineKeyboardButton

	getText := func(key string) string {
		if tm != nil {
			return tm.GetText(language, key)
		}
		return key
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{
			Text:         getText("winback_activate_button"),
			CallbackData: CallbackWinbackActivate,
		},
	})

	return keyboard
}
