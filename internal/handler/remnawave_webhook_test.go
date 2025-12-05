package handler

import (
	"context"
	"os"
	"testing"
	"testing/quick"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/yookasa"
)

func init() {
	// Устанавливаем минимальные переменные окружения для тестов
	os.Setenv("DISABLE_ENV_FILE", "true")
	os.Setenv("ADMIN_TELEGRAM_ID", "123456")
	os.Setenv("TELEGRAM_TOKEN", "test-token")
	os.Setenv("REMNAWAVE_URL", "http://test")
	os.Setenv("REMNAWAVE_TOKEN", "test-token")
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	os.Setenv("TRIAL_TRAFFIC_LIMIT", "10")
	os.Setenv("TRIAL_DAYS", "3")
	os.Setenv("PRICE_1", "500")
	os.Setenv("PRICE_3", "1200")
	os.Setenv("PRICE_6", "2000")
	os.Setenv("PRICE_12", "3500")
	os.Setenv("TRAFFIC_LIMIT", "100")
	os.Setenv("REFERRAL_DAYS", "7")
	os.Setenv("DAYS_IN_MONTH", "30")
	config.InitConfig()
}

// **Feature: recurring-payments, Property 4: Subscription extension after successful recurring payment**
// **Validates: Requirements 2.3**
// *For any* успешный автоплатёж, подписка пользователя должна быть продлена на recurring_months месяцев

// mockCustomerRepo реализует customerRepository для тестов
type mockCustomerRepo struct {
	customer              *database.Customer
	disableRecurringCalls int
	updateNotifiedCalls   int
}

func (m *mockCustomerRepo) FindByTelegramId(ctx context.Context, telegramId int64) (*database.Customer, error) {
	return m.customer, nil
}

func (m *mockCustomerRepo) UpdateWinbackOffer(ctx context.Context, id int64, sentAt, expiresAt time.Time, price, devices, months int) error {
	return nil
}

func (m *mockCustomerRepo) UpdateRecurringNotifiedAt(ctx context.Context, id int64, notifiedAt time.Time) error {
	m.updateNotifiedCalls++
	return nil
}

func (m *mockCustomerRepo) DisableRecurring(ctx context.Context, id int64) error {
	m.disableRecurringCalls++
	return nil
}

// mockPurchaseRepo реализует purchaseRepository для тестов
type mockPurchaseRepo struct {
	hasRecentPurchase bool
}

func (m *mockPurchaseRepo) HasPaidPurchases(ctx context.Context, customerID int64) (bool, error) {
	return false, nil
}

func (m *mockPurchaseRepo) HasRecentPaidPurchase(ctx context.Context, customerID int64, withinMinutes int) (bool, error) {
	return m.hasRecentPurchase, nil
}

// mockTranslationManager реализует translationManager для тестов
type mockTranslationManager struct{}

func (m *mockTranslationManager) GetText(langCode, key string) string {
	return key // Возвращаем ключ как текст для тестов
}

// mockTelegramBot реализует telegramBotClient для тестов
type mockTelegramBot struct {
	sendMessageCalls int
}

func (m *mockTelegramBot) SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	m.sendMessageCalls++
	return &models.Message{}, nil
}

// mockYookasaClient реализует yookasaClient для тестов
type mockYookasaClient struct {
	returnPayment *yookasa.Payment
	returnError   error
	lastAmount    int
	lastMonths    int
}

func (m *mockYookasaClient) CreateRecurringPayment(ctx context.Context, paymentMethodID uuid.UUID, amount int, months int, customerId int64, description string) (*yookasa.Payment, error) {
	m.lastAmount = amount
	m.lastMonths = months
	return m.returnPayment, m.returnError
}

// mockRemnawaveClient реализует remnawaveClient для тестов
type mockRemnawaveClient struct {
	lastDays        int
	lastDeviceLimit *int
	callCount       int
}

func (m *mockRemnawaveClient) CreateOrUpdateUserWithDeviceLimit(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, days int, isTrialUser bool, deviceLimit *int, forceDeviceLimit bool) (*remapi.UserResponseResponse, error) {
	m.lastDays = days
	m.lastDeviceLimit = deviceLimit
	m.callCount++
	return &remapi.UserResponseResponse{}, nil
}

func TestSubscriptionExtensionAfterSuccessfulRecurringPayment(t *testing.T) {
	f := func(
		customerIdRaw uint32,
		telegramIdRaw uint32,
		recurringMonthsRaw uint8,
		recurringAmountRaw uint16,
		paymentMethodIDBytes [16]byte,
	) bool {
		// Ограничиваем входные данные разумными значениями
		customerId := int64(customerIdRaw%1000000) + 1
		telegramId := int64(telegramIdRaw%1000000) + 1
		recurringMonths := int(recurringMonthsRaw%12) + 1 // 1-12 месяцев
		recurringAmount := int(recurringAmountRaw%10000) + 100 // 100-10100 рублей

		// Генерируем UUID из байтов
		paymentMethodID, err := uuid.FromBytes(paymentMethodIDBytes[:])
		if err != nil {
			paymentMethodID = uuid.New()
		}
		paymentMethodIDStr := paymentMethodID.String()

		// Создаём customer с включённым автопродлением
		customer := &database.Customer{
			ID:                  customerId,
			TelegramID:          telegramId,
			RecurringEnabled:    true,
			PaymentMethodID:     &paymentMethodIDStr,
			RecurringMonths:     &recurringMonths,
			RecurringAmount:     &recurringAmount,
			Language:            "ru",
		}

		// Создаём успешный платёж
		successPayment := &yookasa.Payment{
			ID:     uuid.New(),
			Status: "succeeded",
			Paid:   true,
		}

		// Создаём моки
		customerRepo := &mockCustomerRepo{customer: customer}
		purchaseRepo := &mockPurchaseRepo{}
		yookasaClient := &mockYookasaClient{returnPayment: successPayment}
		remnawaveClient := &mockRemnawaveClient{}
		tm := &mockTranslationManager{}
		telegramBot := &mockTelegramBot{}

		// Создаём handler с моками
		handler := &RemnawaveWebhookHandler{
			tm:           tm,
			telegramBot:  telegramBot,
			customerRepo: customerRepo,
			purchaseRepo: purchaseRepo,
			yookasa:      yookasaClient,
			remnawave:    remnawaveClient,
		}

		// Вызываем processRecurringPayment
		ctx := context.Background()
		err = handler.processRecurringPayment(ctx, customer, telegramId, "ru")
		if err != nil {
			t.Logf("processRecurringPayment failed: %v", err)
			return false
		}

		// PROPERTY: Remnawave должен быть вызван для продления подписки
		if remnawaveClient.callCount != 1 {
			t.Logf("Expected 1 call to CreateOrUpdateUserWithDeviceLimit, got %d", remnawaveClient.callCount)
			return false
		}

		// PROPERTY: Количество дней должно соответствовать recurring_months * DAYS_IN_MONTH (30 по умолчанию)
		expectedDays := recurringMonths * 30 // DaysInMonth() возвращает 30 по умолчанию
		if remnawaveClient.lastDays != expectedDays {
			t.Logf("Expected %d days, got %d days (months=%d)", expectedDays, remnawaveClient.lastDays, recurringMonths)
			return false
		}

		// PROPERTY: YooKassa должен быть вызван с правильными параметрами
		if yookasaClient.lastAmount != recurringAmount {
			t.Logf("Expected amount %d, got %d", recurringAmount, yookasaClient.lastAmount)
			return false
		}

		if yookasaClient.lastMonths != recurringMonths {
			t.Logf("Expected months %d, got %d", recurringMonths, yookasaClient.lastMonths)
			return false
		}

		// PROPERTY: Уведомление об успешном продлении должно быть отправлено
		if telegramBot.sendMessageCalls != 1 {
			t.Logf("Expected 1 SendMessage call, got %d", telegramBot.sendMessageCalls)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// **Feature: recurring-payments, Property 5: Recurring disable on permission_revoked**
// **Validates: Requirements 2.5**
// *For any* автоплатёж отклонённый с причиной permission_revoked, поле recurring_enabled должно стать false и payment_method_id должен быть очищен

func TestRecurringDisableOnPermissionRevoked(t *testing.T) {
	f := func(
		customerIdRaw uint32,
		telegramIdRaw uint32,
		recurringMonthsRaw uint8,
		recurringAmountRaw uint16,
		paymentMethodIDBytes [16]byte,
	) bool {
		// Ограничиваем входные данные разумными значениями
		customerId := int64(customerIdRaw%1000000) + 1
		telegramId := int64(telegramIdRaw%1000000) + 1
		recurringMonths := int(recurringMonthsRaw%12) + 1 // 1-12 месяцев
		recurringAmount := int(recurringAmountRaw%10000) + 100 // 100-10100 рублей

		// Генерируем UUID из байтов
		paymentMethodID, err := uuid.FromBytes(paymentMethodIDBytes[:])
		if err != nil {
			paymentMethodID = uuid.New()
		}
		paymentMethodIDStr := paymentMethodID.String()

		// Создаём customer с включённым автопродлением
		customer := &database.Customer{
			ID:               customerId,
			TelegramID:       telegramId,
			RecurringEnabled: true,
			PaymentMethodID:  &paymentMethodIDStr,
			RecurringMonths:  &recurringMonths,
			RecurringAmount:  &recurringAmount,
			Language:         "ru",
		}

		// Создаём платёж отклонённый с причиной permission_revoked
		cancelledPayment := &yookasa.Payment{
			ID:     uuid.New(),
			Status: "canceled",
			Paid:   false,
			CancellationDetails: &yookasa.CancellationDetails{
				Party:  "yoo_money",
				Reason: "permission_revoked",
			},
		}

		// Создаём моки
		customerRepo := &mockCustomerRepo{customer: customer}
		purchaseRepo := &mockPurchaseRepo{}
		yookasaClient := &mockYookasaClient{returnPayment: cancelledPayment}
		remnawaveClient := &mockRemnawaveClient{}
		tm := &mockTranslationManager{}
		telegramBot := &mockTelegramBot{}

		// Создаём handler с моками
		handler := &RemnawaveWebhookHandler{
			tm:           tm,
			telegramBot:  telegramBot,
			customerRepo: customerRepo,
			purchaseRepo: purchaseRepo,
			yookasa:      yookasaClient,
			remnawave:    remnawaveClient,
		}

		// Вызываем processRecurringPayment
		ctx := context.Background()
		err = handler.processRecurringPayment(ctx, customer, telegramId, "ru")
		
		// При permission_revoked ошибка не возвращается (обрабатывается внутри)
		if err != nil {
			t.Logf("processRecurringPayment returned unexpected error: %v", err)
			return false
		}

		// PROPERTY: DisableRecurring должен быть вызван ровно 1 раз
		if customerRepo.disableRecurringCalls != 1 {
			t.Logf("Expected 1 call to DisableRecurring, got %d", customerRepo.disableRecurringCalls)
			return false
		}

		// PROPERTY: Remnawave НЕ должен быть вызван (подписка не продлевается)
		if remnawaveClient.callCount != 0 {
			t.Logf("Expected 0 calls to CreateOrUpdateUserWithDeviceLimit, got %d", remnawaveClient.callCount)
			return false
		}

		// PROPERTY: Уведомление об отзыве разрешения должно быть отправлено
		if telegramBot.sendMessageCalls != 1 {
			t.Logf("Expected 1 SendMessage call for permission_revoked notification, got %d", telegramBot.sendMessageCalls)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// TestRecurringDisableOnPermissionRevokedExamples - примеры для конкретных случаев permission_revoked
func TestRecurringDisableOnPermissionRevokedExamples(t *testing.T) {
	tests := []struct {
		name            string
		recurringMonths int
		recurringAmount int
		cancellationParty string
	}{
		{
			name:              "permission revoked by yoo_money",
			recurringMonths:   1,
			recurringAmount:   500,
			cancellationParty: "yoo_money",
		},
		{
			name:              "permission revoked by payment_network",
			recurringMonths:   3,
			recurringAmount:   1200,
			cancellationParty: "payment_network",
		},
		{
			name:              "permission revoked by merchant",
			recurringMonths:   6,
			recurringAmount:   2000,
			cancellationParty: "merchant",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paymentMethodID := uuid.New().String()
			customer := &database.Customer{
				ID:               1,
				TelegramID:       123456,
				RecurringEnabled: true,
				PaymentMethodID:  &paymentMethodID,
				RecurringMonths:  &tt.recurringMonths,
				RecurringAmount:  &tt.recurringAmount,
				Language:         "ru",
			}

			cancelledPayment := &yookasa.Payment{
				ID:     uuid.New(),
				Status: "canceled",
				Paid:   false,
				CancellationDetails: &yookasa.CancellationDetails{
					Party:  tt.cancellationParty,
					Reason: "permission_revoked",
				},
			}

			customerRepo := &mockCustomerRepo{customer: customer}
			purchaseRepo := &mockPurchaseRepo{}
			yookasaClient := &mockYookasaClient{returnPayment: cancelledPayment}
			remnawaveClient := &mockRemnawaveClient{}
			tm := &mockTranslationManager{}
			telegramBot := &mockTelegramBot{}

			handler := &RemnawaveWebhookHandler{
				tm:           tm,
				telegramBot:  telegramBot,
				customerRepo: customerRepo,
				purchaseRepo: purchaseRepo,
				yookasa:      yookasaClient,
				remnawave:    remnawaveClient,
			}

			ctx := context.Background()
			err := handler.processRecurringPayment(ctx, customer, customer.TelegramID, "ru")
			if err != nil {
				t.Fatalf("processRecurringPayment failed: %v", err)
			}

			// Проверяем что DisableRecurring был вызван
			if customerRepo.disableRecurringCalls != 1 {
				t.Errorf("Expected 1 call to DisableRecurring, got %d", customerRepo.disableRecurringCalls)
			}

			// Проверяем что подписка НЕ была продлена
			if remnawaveClient.callCount != 0 {
				t.Errorf("Expected 0 calls to CreateOrUpdateUserWithDeviceLimit, got %d", remnawaveClient.callCount)
			}

			// Проверяем что уведомление было отправлено
			if telegramBot.sendMessageCalls != 1 {
				t.Errorf("Expected 1 SendMessage call, got %d", telegramBot.sendMessageCalls)
			}
		})
	}
}

// TestSubscriptionExtensionExamples - примеры для конкретных случаев
func TestSubscriptionExtensionExamples(t *testing.T) {
	tests := []struct {
		name            string
		recurringMonths int
		recurringAmount int
		expectedDays    int
	}{
		{
			name:            "1 month subscription",
			recurringMonths: 1,
			recurringAmount: 500,
			expectedDays:    30,
		},
		{
			name:            "3 months subscription",
			recurringMonths: 3,
			recurringAmount: 1200,
			expectedDays:    90,
		},
		{
			name:            "6 months subscription",
			recurringMonths: 6,
			recurringAmount: 2000,
			expectedDays:    180,
		},
		{
			name:            "12 months subscription",
			recurringMonths: 12,
			recurringAmount: 3500,
			expectedDays:    360,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paymentMethodID := uuid.New().String()
			customer := &database.Customer{
				ID:               1,
				TelegramID:       123456,
				RecurringEnabled: true,
				PaymentMethodID:  &paymentMethodID,
				RecurringMonths:  &tt.recurringMonths,
				RecurringAmount:  &tt.recurringAmount,
				Language:         "ru",
			}

			successPayment := &yookasa.Payment{
				ID:     uuid.New(),
				Status: "succeeded",
				Paid:   true,
			}

			customerRepo := &mockCustomerRepo{customer: customer}
			purchaseRepo := &mockPurchaseRepo{}
			yookasaClient := &mockYookasaClient{returnPayment: successPayment}
			remnawaveClient := &mockRemnawaveClient{}
			tm := &mockTranslationManager{}
			telegramBot := &mockTelegramBot{}

			handler := &RemnawaveWebhookHandler{
				tm:           tm,
				telegramBot:  telegramBot,
				customerRepo: customerRepo,
				purchaseRepo: purchaseRepo,
				yookasa:      yookasaClient,
				remnawave:    remnawaveClient,
			}

			ctx := context.Background()
			err := handler.processRecurringPayment(ctx, customer, customer.TelegramID, "ru")
			if err != nil {
				t.Fatalf("processRecurringPayment failed: %v", err)
			}

			if remnawaveClient.lastDays != tt.expectedDays {
				t.Errorf("Expected %d days, got %d", tt.expectedDays, remnawaveClient.lastDays)
			}

			if yookasaClient.lastMonths != tt.recurringMonths {
				t.Errorf("Expected %d months in YooKassa call, got %d", tt.recurringMonths, yookasaClient.lastMonths)
			}
		})
	}
}


// **Feature: recurring-payments, Property: Race condition protection**
// **Validates: Requirements 2.3**
// *For any* автоплатёж, если был недавний платёж (< 5 минут), новый платёж не создаётся

func TestRecurringPaymentRaceConditionProtection(t *testing.T) {
	paymentMethodID := uuid.New().String()
	recurringMonths := 1
	recurringAmount := 500
	
	customer := &database.Customer{
		ID:               1,
		TelegramID:       123456,
		RecurringEnabled: true,
		PaymentMethodID:  &paymentMethodID,
		RecurringMonths:  &recurringMonths,
		RecurringAmount:  &recurringAmount,
		Language:         "ru",
	}

	successPayment := &yookasa.Payment{
		ID:     uuid.New(),
		Status: "succeeded",
		Paid:   true,
	}

	// Mock с hasRecentPurchase = true (был недавний платёж)
	customerRepo := &mockCustomerRepo{customer: customer}
	purchaseRepo := &mockPurchaseRepo{hasRecentPurchase: true}
	yookasaClient := &mockYookasaClient{returnPayment: successPayment}
	remnawaveClient := &mockRemnawaveClient{}
	tm := &mockTranslationManager{}
	telegramBot := &mockTelegramBot{}

	handler := &RemnawaveWebhookHandler{
		tm:           tm,
		telegramBot:  telegramBot,
		customerRepo: customerRepo,
		purchaseRepo: purchaseRepo,
		yookasa:      yookasaClient,
		remnawave:    remnawaveClient,
	}

	ctx := context.Background()
	err := handler.processRecurringPayment(ctx, customer, customer.TelegramID, "ru")
	
	// Ошибки быть не должно
	if err != nil {
		t.Fatalf("processRecurringPayment failed: %v", err)
	}

	// PROPERTY: YooKassa НЕ должен быть вызван (защита от race condition)
	if yookasaClient.lastAmount != 0 {
		t.Errorf("Expected no YooKassa call due to race condition protection, but got amount=%d", yookasaClient.lastAmount)
	}

	// PROPERTY: Remnawave НЕ должен быть вызван
	if remnawaveClient.callCount != 0 {
		t.Errorf("Expected 0 calls to CreateOrUpdateUserWithDeviceLimit, got %d", remnawaveClient.callCount)
	}

	// PROPERTY: Уведомление НЕ должно быть отправлено
	if telegramBot.sendMessageCalls != 0 {
		t.Errorf("Expected 0 SendMessage calls, got %d", telegramBot.sendMessageCalls)
	}
}

func TestRecurringPaymentNoRecentPurchase(t *testing.T) {
	paymentMethodID := uuid.New().String()
	recurringMonths := 1
	recurringAmount := 500
	
	customer := &database.Customer{
		ID:               1,
		TelegramID:       123456,
		RecurringEnabled: true,
		PaymentMethodID:  &paymentMethodID,
		RecurringMonths:  &recurringMonths,
		RecurringAmount:  &recurringAmount,
		Language:         "ru",
	}

	successPayment := &yookasa.Payment{
		ID:     uuid.New(),
		Status: "succeeded",
		Paid:   true,
	}

	// Mock с hasRecentPurchase = false (не было недавнего платежа)
	customerRepo := &mockCustomerRepo{customer: customer}
	purchaseRepo := &mockPurchaseRepo{hasRecentPurchase: false}
	yookasaClient := &mockYookasaClient{returnPayment: successPayment}
	remnawaveClient := &mockRemnawaveClient{}
	tm := &mockTranslationManager{}
	telegramBot := &mockTelegramBot{}

	handler := &RemnawaveWebhookHandler{
		tm:           tm,
		telegramBot:  telegramBot,
		customerRepo: customerRepo,
		purchaseRepo: purchaseRepo,
		yookasa:      yookasaClient,
		remnawave:    remnawaveClient,
	}

	ctx := context.Background()
	err := handler.processRecurringPayment(ctx, customer, customer.TelegramID, "ru")
	
	if err != nil {
		t.Fatalf("processRecurringPayment failed: %v", err)
	}

	// PROPERTY: YooKassa ДОЛЖЕН быть вызван
	if yookasaClient.lastAmount != recurringAmount {
		t.Errorf("Expected YooKassa call with amount=%d, got %d", recurringAmount, yookasaClient.lastAmount)
	}

	// PROPERTY: Remnawave ДОЛЖЕН быть вызван
	if remnawaveClient.callCount != 1 {
		t.Errorf("Expected 1 call to CreateOrUpdateUserWithDeviceLimit, got %d", remnawaveClient.callCount)
	}

	// PROPERTY: Уведомление ДОЛЖНО быть отправлено
	if telegramBot.sendMessageCalls != 1 {
		t.Errorf("Expected 1 SendMessage call, got %d", telegramBot.sendMessageCalls)
	}
}
