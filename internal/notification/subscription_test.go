package notification

import (
	"context"
	"testing"
	"testing/quick"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

type customerRepoMock struct {
	customers                  *[]database.Customer
	trialUsersForNotification  []database.Customer
	expiredTrialUsersForWinback []database.Customer
	err                        error
	trialNotificationErr       error
	winbackErr                 error
	updateNotifiedAtCalls      int
	updateNotifiedAtIDs        []int64
	updateWinbackCalls         int
	updateWinbackIDs           []int64
}

func (m *customerRepoMock) FindByExpirationRange(ctx context.Context, startDate, endDate time.Time) (*[]database.Customer, error) {
	return m.customers, m.err
}

func (m *customerRepoMock) FindTrialUsersForInactiveNotification(ctx context.Context) ([]database.Customer, error) {
	return m.trialUsersForNotification, m.trialNotificationErr
}

func (m *customerRepoMock) UpdateTrialInactiveNotifiedAt(ctx context.Context, id int64, notifiedAt time.Time) error {
	m.updateNotifiedAtCalls++
	m.updateNotifiedAtIDs = append(m.updateNotifiedAtIDs, id)
	return nil
}

func (m *customerRepoMock) FindExpiredTrialUsersForWinback(ctx context.Context) ([]database.Customer, error) {
	return m.expiredTrialUsersForWinback, m.winbackErr
}

func (m *customerRepoMock) UpdateWinbackOffer(ctx context.Context, id int64, sentAt, expiresAt time.Time, price, devices, months int) error {
	m.updateWinbackCalls++
	m.updateWinbackIDs = append(m.updateWinbackIDs, id)
	return nil
}

type purchaseRepoMock struct {
	tributes    *[]database.Purchase
	err         error
	receivedIDs []int64
}

func (m *purchaseRepoMock) FindLatestActiveTributesByCustomerIDs(ctx context.Context, customerIDs []int64) (*[]database.Purchase, error) {
	m.receivedIDs = append([]int64(nil), customerIDs...)
	return m.tributes, m.err
}

type paymentServiceMock struct {
	createCalls        int
	processCalls       int
	amounts            []float64
	months             []int
	customers          []int64
	processIDs         []int64
	createErr          error
	processErr         error
	purchaseIDToReturn int64
}

func (m *paymentServiceMock) CreatePurchase(ctx context.Context, amount float64, months int, customer *database.Customer, invoiceType database.InvoiceType) (string, int64, error) {
	m.createCalls++
	m.amounts = append(m.amounts, amount)
	m.months = append(m.months, months)
	if customer != nil {
		m.customers = append(m.customers, customer.ID)
	}
	if m.purchaseIDToReturn == 0 {
		m.purchaseIDToReturn = int64(m.createCalls)
	}
	return "", m.purchaseIDToReturn, m.createErr
}

func (m *paymentServiceMock) ProcessPurchaseById(ctx context.Context, purchaseId int64) error {
	m.processCalls++
	m.processIDs = append(m.processIDs, purchaseId)
	return m.processErr
}

// **Feature: trial-notifications, Property 2: Inactive Notification Eligibility**
// **Validates: Requirements 2.1, 2.3, 2.4**
// *For any* trial customer, ShouldSendInactiveNotification SHALL return true only when:
// trial started >= 1 hour ago AND firstConnectedAt == nil AND TrialInactiveNotifiedAt is nil.

func TestShouldSendInactiveNotificationProperty(t *testing.T) {
	// Property: функция возвращает true ТОЛЬКО когда все три условия выполнены:
	// 1. TrialInactiveNotifiedAt == nil (уведомление ещё не отправлялось)
	// 2. firstConnectedAt == nil (пользователь ещё не подключался)
	// 3. CreatedAt <= now - 1 hour (триал начался >= 1 час назад)

	f := func(
		hoursAgo uint16, // сколько часов назад создан customer (0-1000)
		alreadyNotified bool, // было ли уже отправлено уведомление
		alreadyConnected bool, // подключался ли пользователь
	) bool {
		// Ограничиваем hoursAgo разумным диапазоном
		hoursAgoVal := int(hoursAgo % 1000)

		now := time.Now()
		createdAt := now.Add(-time.Duration(hoursAgoVal) * time.Hour)

		customer := &database.Customer{
			ID:        1,
			CreatedAt: createdAt,
		}

		// Устанавливаем TrialInactiveNotifiedAt если уже было уведомление
		if alreadyNotified {
			notifiedAt := now.Add(-30 * time.Minute)
			customer.TrialInactiveNotifiedAt = &notifiedAt
		}

		// Устанавливаем firstConnectedAt если пользователь уже подключался
		var firstConnectedAt *time.Time
		if alreadyConnected {
			connectedAt := now.Add(-45 * time.Minute)
			firstConnectedAt = &connectedAt
		}

		result := ShouldSendInactiveNotification(customer, firstConnectedAt, now)

		// Вычисляем ожидаемый результат по спецификации
		trialStartedMoreThanHourAgo := hoursAgoVal >= 1
		notYetNotified := !alreadyNotified
		notYetConnected := !alreadyConnected

		expected := trialStartedMoreThanHourAgo && notYetNotified && notYetConnected

		if result != expected {
			t.Logf("Mismatch: hoursAgo=%d, alreadyNotified=%v, alreadyConnected=%v",
				hoursAgoVal, alreadyNotified, alreadyConnected)
			t.Logf("Expected: %v, Got: %v", expected, result)
			t.Logf("Conditions: trialStarted>1h=%v, notNotified=%v, notConnected=%v",
				trialStartedMoreThanHourAgo, notYetNotified, notYetConnected)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// TestShouldSendInactiveNotification_EdgeCases проверяет граничные случаи
func TestShouldSendInactiveNotification_EdgeCases(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name             string
		hoursAgo         int
		alreadyNotified  bool
		alreadyConnected bool
		expected         bool
	}{
		{
			name:             "exactly 1 hour ago, not notified, not connected - should send",
			hoursAgo:         1,
			alreadyNotified:  false,
			alreadyConnected: false,
			expected:         true,
		},
		{
			name:             "59 minutes ago - should not send (less than 1 hour)",
			hoursAgo:         0, // будет 59 минут
			alreadyNotified:  false,
			alreadyConnected: false,
			expected:         false,
		},
		{
			name:             "2 hours ago, already notified - should not send",
			hoursAgo:         2,
			alreadyNotified:  true,
			alreadyConnected: false,
			expected:         false,
		},
		{
			name:             "2 hours ago, already connected - should not send",
			hoursAgo:         2,
			alreadyNotified:  false,
			alreadyConnected: true,
			expected:         false,
		},
		{
			name:             "24 hours ago, not notified, not connected - should send",
			hoursAgo:         24,
			alreadyNotified:  false,
			alreadyConnected: false,
			expected:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var createdAt time.Time
			if tt.hoursAgo == 0 {
				// Специальный случай: 59 минут назад
				createdAt = now.Add(-59 * time.Minute)
			} else {
				createdAt = now.Add(-time.Duration(tt.hoursAgo) * time.Hour)
			}

			customer := &database.Customer{
				ID:        1,
				CreatedAt: createdAt,
			}

			if tt.alreadyNotified {
				notifiedAt := now.Add(-30 * time.Minute)
				customer.TrialInactiveNotifiedAt = &notifiedAt
			}

			var firstConnectedAt *time.Time
			if tt.alreadyConnected {
				connectedAt := now.Add(-45 * time.Minute)
				firstConnectedAt = &connectedAt
			}

			result := ShouldSendInactiveNotification(customer, firstConnectedAt, now)

			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}


// **Feature: trial-notifications, Property 5: Inactive Notification Message Contains MiniApp Button**
// **Validates: Requirements 2.2**
// *For any* inactive notification message, the generated keyboard SHALL contain a button with MiniApp URL
// when MiniApp URL is configured, or a fallback callback button when not configured.

func TestBuildInactiveNotificationKeyboardProperty(t *testing.T) {
	// Property: когда miniAppURL не пустой, клавиатура содержит кнопку с WebApp URL
	// когда miniAppURL пустой, клавиатура содержит fallback callback кнопку

	// Используем nil для translation manager - функция должна обрабатывать это
	// и возвращать ключи как есть

	f := func(
		urlLength uint8, // длина URL (0 = пустой URL)
		language uint8,  // индекс языка
	) bool {
		// Генерируем URL разной длины
		var miniAppURL string
		if urlLength > 0 {
			// Генерируем валидный URL
			miniAppURL = "https://example.com/app"
			// Добавляем случайный путь для разнообразия
			for i := uint8(0); i < urlLength%10; i++ {
				miniAppURL += "/path"
			}
		}

		// Выбираем язык
		languages := []string{"ru", "en"}
		lang := languages[int(language)%len(languages)]

		// Вызываем тестируемую функцию с nil translation manager
		keyboard := BuildInactiveNotificationKeyboardWithURL(lang, nil, miniAppURL)

		// Проверяем результат
		if len(keyboard) == 0 {
			t.Log("Keyboard should not be empty")
			return false
		}

		if len(keyboard[0]) == 0 {
			t.Log("First row should not be empty")
			return false
		}

		button := keyboard[0][0]

		if miniAppURL != "" {
			// Когда URL настроен - должна быть WebApp кнопка
			if button.WebApp == nil {
				t.Logf("Expected WebApp button when URL is set, got CallbackData: %s", button.CallbackData)
				return false
			}
			if button.WebApp.URL != miniAppURL {
				t.Logf("Expected WebApp URL %s, got %s", miniAppURL, button.WebApp.URL)
				return false
			}
			// Текст кнопки должен быть ключом (т.к. tm == nil)
			if button.Text != "your_subscription_button" {
				t.Logf("Expected button text 'your_subscription_button', got '%s'", button.Text)
				return false
			}
		} else {
			// Когда URL не настроен - должна быть callback кнопка
			if button.WebApp != nil {
				t.Log("Expected callback button when URL is empty, got WebApp button")
				return false
			}
			if button.CallbackData != "connect" {
				t.Logf("Expected CallbackData 'connect', got '%s'", button.CallbackData)
				return false
			}
			// Текст кнопки должен быть ключом (т.к. tm == nil)
			if button.Text != "connect_button" {
				t.Logf("Expected button text 'connect_button', got '%s'", button.Text)
				return false
			}
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// **Feature: trial-notifications, Property 3: Winback Offer Eligibility**
// **Validates: Requirements 3.1, 3.3**
// *For any* trial customer with expired subscription, ShouldSendWinbackOffer SHALL return true only when:
// expiry was >= 24 hours ago AND WinbackOfferSentAt is nil.

func TestShouldSendWinbackOfferProperty(t *testing.T) {
	// Property: функция возвращает true ТОЛЬКО когда все условия выполнены:
	// 1. WinbackOfferSentAt == nil (предложение ещё не отправлялось)
	// 2. ExpireAt != nil (есть дата истечения)
	// 3. ExpireAt <= now - 24 hours (триал истёк >= 24 часа назад)

	f := func(
		hoursExpiredAgo uint16, // сколько часов назад истёк триал (0-1000)
		alreadySent bool,       // было ли уже отправлено предложение
		hasExpireAt bool,       // есть ли дата истечения
	) bool {
		// Ограничиваем hoursExpiredAgo разумным диапазоном
		hoursExpiredAgoVal := int(hoursExpiredAgo % 1000)

		now := time.Now()

		customer := &database.Customer{
			ID: 1,
		}

		// Устанавливаем ExpireAt если есть
		if hasExpireAt {
			expireAt := now.Add(-time.Duration(hoursExpiredAgoVal) * time.Hour)
			customer.ExpireAt = &expireAt
		}

		// Устанавливаем WinbackOfferSentAt если уже было отправлено
		if alreadySent {
			sentAt := now.Add(-12 * time.Hour)
			customer.WinbackOfferSentAt = &sentAt
		}

		result := ShouldSendWinbackOffer(customer, now)

		// Вычисляем ожидаемый результат по спецификации
		hasExpiration := hasExpireAt
		expiredMoreThan24HoursAgo := hasExpireAt && hoursExpiredAgoVal >= 24
		notYetSent := !alreadySent

		expected := hasExpiration && expiredMoreThan24HoursAgo && notYetSent

		if result != expected {
			t.Logf("Mismatch: hoursExpiredAgo=%d, alreadySent=%v, hasExpireAt=%v",
				hoursExpiredAgoVal, alreadySent, hasExpireAt)
			t.Logf("Expected: %v, Got: %v", expected, result)
			t.Logf("Conditions: hasExpiration=%v, expired>24h=%v, notSent=%v",
				hasExpiration, expiredMoreThan24HoursAgo, notYetSent)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// TestShouldSendWinbackOffer_EdgeCases проверяет граничные случаи
func TestShouldSendWinbackOffer_EdgeCases(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		hoursAgo    int
		alreadySent bool
		hasExpireAt bool
		expected    bool
	}{
		{
			name:        "exactly 24 hours ago, not sent, has expireAt - should send",
			hoursAgo:    24,
			alreadySent: false,
			hasExpireAt: true,
			expected:    true,
		},
		{
			name:        "23 hours ago - should not send (less than 24 hours)",
			hoursAgo:    23,
			alreadySent: false,
			hasExpireAt: true,
			expected:    false,
		},
		{
			name:        "48 hours ago, already sent - should not send",
			hoursAgo:    48,
			alreadySent: true,
			hasExpireAt: true,
			expected:    false,
		},
		{
			name:        "48 hours ago, no expireAt - should not send",
			hoursAgo:    48,
			alreadySent: false,
			hasExpireAt: false,
			expected:    false,
		},
		{
			name:        "72 hours ago, not sent, has expireAt - should send",
			hoursAgo:    72,
			alreadySent: false,
			hasExpireAt: true,
			expected:    true,
		},
		{
			name:        "0 hours ago (just expired) - should not send",
			hoursAgo:    0,
			alreadySent: false,
			hasExpireAt: true,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			customer := &database.Customer{
				ID: 1,
			}

			if tt.hasExpireAt {
				expireAt := now.Add(-time.Duration(tt.hoursAgo) * time.Hour)
				customer.ExpireAt = &expireAt
			}

			if tt.alreadySent {
				sentAt := now.Add(-12 * time.Hour)
				customer.WinbackOfferSentAt = &sentAt
			}

			result := ShouldSendWinbackOffer(customer, now)

			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestBuildInactiveNotificationKeyboard_EdgeCases проверяет граничные случаи
func TestBuildInactiveNotificationKeyboard_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		miniAppURL     string
		expectWebApp   bool
		expectedURL    string
		expectedText   string
	}{
		{
			name:         "empty URL - fallback to callback",
			miniAppURL:   "",
			expectWebApp: false,
			expectedText: "connect_button",
		},
		{
			name:         "valid URL - WebApp button",
			miniAppURL:   "https://t.me/mybot/app",
			expectWebApp: true,
			expectedURL:  "https://t.me/mybot/app",
			expectedText: "your_subscription_button",
		},
		{
			name:         "URL with query params",
			miniAppURL:   "https://example.com/app?param=value",
			expectWebApp: true,
			expectedURL:  "https://example.com/app?param=value",
			expectedText: "your_subscription_button",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyboard := BuildInactiveNotificationKeyboardWithURL("ru", nil, tt.miniAppURL)

			if len(keyboard) == 0 || len(keyboard[0]) == 0 {
				t.Fatal("Keyboard should have at least one button")
			}

			button := keyboard[0][0]

			if tt.expectWebApp {
				if button.WebApp == nil {
					t.Error("Expected WebApp button")
				} else if button.WebApp.URL != tt.expectedURL {
					t.Errorf("Expected URL %s, got %s", tt.expectedURL, button.WebApp.URL)
				}
			} else {
				if button.WebApp != nil {
					t.Error("Expected callback button, got WebApp")
				}
				if button.CallbackData != "connect" {
					t.Errorf("Expected CallbackData 'connect', got '%s'", button.CallbackData)
				}
			}

			if button.Text != tt.expectedText {
				t.Errorf("Expected text '%s', got '%s'", tt.expectedText, button.Text)
			}
		})
	}
}
