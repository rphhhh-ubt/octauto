package handler

import (
	"testing"
	"testing/quick"
	"time"
)

// **Feature: trial-notifications, Property 4: Winback Offer Activation Validity**
// **Validates: Requirements 3.4, 3.5**
// *For any* winback offer activation attempt, the offer SHALL be valid only when
// WinbackOfferExpiresAt > current time.

func TestIsWinbackOfferValidProperty(t *testing.T) {
	// Property: IsWinbackOfferValid возвращает true ТОЛЬКО когда:
	// 1. expiresAt != nil
	// 2. expiresAt > currentTime

	f := func(
		offsetMinutes int32, // смещение от текущего времени в минутах (-10000 до +10000)
		hasExpiresAt bool,   // есть ли дата истечения
	) bool {
		// Ограничиваем смещение разумным диапазоном
		offsetVal := int(offsetMinutes % 10000)

		currentTime := time.Now()

		var expiresAt *time.Time
		if hasExpiresAt {
			expTime := currentTime.Add(time.Duration(offsetVal) * time.Minute)
			expiresAt = &expTime
		}

		result := IsWinbackOfferValid(expiresAt, currentTime)

		// Вычисляем ожидаемый результат по спецификации:
		// Предложение действительно только когда expiresAt > currentTime
		var expected bool
		if !hasExpiresAt {
			// Если expiresAt == nil, предложение недействительно
			expected = false
		} else {
			// Предложение действительно только если expiresAt > currentTime
			expected = offsetVal > 0
		}

		if result != expected {
			t.Logf("Mismatch: offsetMinutes=%d, hasExpiresAt=%v", offsetVal, hasExpiresAt)
			t.Logf("Expected: %v, Got: %v", expected, result)
			if expiresAt != nil {
				t.Logf("expiresAt=%v, currentTime=%v, diff=%v",
					expiresAt.Format(time.RFC3339),
					currentTime.Format(time.RFC3339),
					expiresAt.Sub(currentTime))
			}
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// TestIsWinbackOfferValid_EdgeCases проверяет граничные случаи
func TestIsWinbackOfferValid_EdgeCases(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		expiresAt *time.Time
		expected  bool
	}{
		{
			name:      "nil expiresAt - should be invalid",
			expiresAt: nil,
			expected:  false,
		},
		{
			name:      "expiresAt in future (1 hour) - should be valid",
			expiresAt: timePtr(now.Add(1 * time.Hour)),
			expected:  true,
		},
		{
			name:      "expiresAt in past (1 hour ago) - should be invalid",
			expiresAt: timePtr(now.Add(-1 * time.Hour)),
			expected:  false,
		},
		{
			name:      "expiresAt exactly now - should be invalid (not strictly after)",
			expiresAt: timePtr(now),
			expected:  false,
		},
		{
			name:      "expiresAt 1 second in future - should be valid",
			expiresAt: timePtr(now.Add(1 * time.Second)),
			expected:  true,
		},
		{
			name:      "expiresAt 1 second in past - should be invalid",
			expiresAt: timePtr(now.Add(-1 * time.Second)),
			expected:  false,
		},
		{
			name:      "expiresAt far in future (48 hours) - should be valid",
			expiresAt: timePtr(now.Add(48 * time.Hour)),
			expected:  true,
		},
		{
			name:      "expiresAt far in past (48 hours ago) - should be invalid",
			expiresAt: timePtr(now.Add(-48 * time.Hour)),
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWinbackOfferValid(tt.expiresAt, now)

			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// timePtr возвращает указатель на time.Time
func timePtr(t time.Time) *time.Time {
	return &t
}

// **Feature: trial-notifications, Property 6: Winback Purchase Uses Offer Device Limit**
// **Validates: Requirements 3.4**
// *For any* activated winback offer, the created purchase SHALL set hwidDeviceLimit
// to WinbackOfferDevices value.

func TestExtractWinbackPurchaseParamsProperty(t *testing.T) {
	// Property: ExtractWinbackPurchaseParams должен:
	// 1. Возвращать IsValid=false если любой параметр nil
	// 2. Возвращать Devices равный WinbackOfferDevices (hwidDeviceLimit)
	// 3. Возвращать Days = Months * daysInMonth

	f := func(
		price int32,
		devices int32,
		months int32,
		daysInMonth int32,
		hasPrice bool,
		hasDevices bool,
		hasMonths bool,
	) bool {
		// Ограничиваем значения разумным диапазоном
		priceVal := int(price%10000 + 1)     // 1-10000 рублей
		devicesVal := int(devices%10 + 1)    // 1-10 устройств
		monthsVal := int(months%12 + 1)      // 1-12 месяцев
		daysInMonthVal := int(daysInMonth%31 + 28) // 28-58 дней в месяце

		// Создаём указатели на параметры
		var pricePtr, devicesPtr, monthsPtr *int
		if hasPrice {
			pricePtr = &priceVal
		}
		if hasDevices {
			devicesPtr = &devicesVal
		}
		if hasMonths {
			monthsPtr = &monthsVal
		}

		result := ExtractWinbackPurchaseParams(pricePtr, devicesPtr, monthsPtr, daysInMonthVal)

		// Проверяем валидность
		expectedValid := hasPrice && hasDevices && hasMonths
		if result.IsValid != expectedValid {
			t.Logf("IsValid mismatch: expected %v, got %v", expectedValid, result.IsValid)
			t.Logf("hasPrice=%v, hasDevices=%v, hasMonths=%v", hasPrice, hasDevices, hasMonths)
			return false
		}

		// Если невалидно - остальные проверки не нужны
		if !result.IsValid {
			return true
		}

		// Property 6: hwidDeviceLimit (Devices) должен быть равен WinbackOfferDevices
		if result.Devices != devicesVal {
			t.Logf("Devices mismatch: expected %d (WinbackOfferDevices), got %d", devicesVal, result.Devices)
			return false
		}

		// Проверяем Price
		if result.Price != priceVal {
			t.Logf("Price mismatch: expected %d, got %d", priceVal, result.Price)
			return false
		}

		// Проверяем Months
		if result.Months != monthsVal {
			t.Logf("Months mismatch: expected %d, got %d", monthsVal, result.Months)
			return false
		}

		// Проверяем Days = Months * daysInMonth
		expectedDays := monthsVal * daysInMonthVal
		if result.Days != expectedDays {
			t.Logf("Days mismatch: expected %d (months=%d * daysInMonth=%d), got %d",
				expectedDays, monthsVal, daysInMonthVal, result.Days)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// TestExtractWinbackPurchaseParams_DeviceLimitPreserved проверяет что device limit
// из winback предложения сохраняется без изменений
func TestExtractWinbackPurchaseParams_DeviceLimitPreserved(t *testing.T) {
	daysInMonth := 30

	tests := []struct {
		name            string
		offerDevices    int
		expectedDevices int
	}{
		{"1 device", 1, 1},
		{"2 devices", 2, 2},
		{"3 devices", 3, 3},
		{"5 devices", 5, 5},
		{"10 devices", 10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			price := 100
			months := 1

			result := ExtractWinbackPurchaseParams(&price, &tt.offerDevices, &months, daysInMonth)

			if !result.IsValid {
				t.Fatal("Expected valid result")
			}

			// Property 6: hwidDeviceLimit должен быть равен WinbackOfferDevices
			if result.Devices != tt.expectedDevices {
				t.Errorf("Devices: expected %d, got %d", tt.expectedDevices, result.Devices)
			}
		})
	}
}

// intPtr возвращает указатель на int
func intPtr(i int) *int {
	return &i
}
