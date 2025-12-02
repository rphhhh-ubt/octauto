package config

import (
	"os"
	"strconv"
	"testing"
	"testing/quick"
)

// **Feature: trial-notifications, Property 1: Winback Configuration Parsing**
// **Validates: Requirements 1.3**
// *For any* valid ENV configuration with WINBACK_PRICE, GetWinbackPrice() SHALL return
// the exact value from ENV (in rubles, no conversion).

func TestWinbackConfigParsingProperty(t *testing.T) {
	// Сохраняем оригинальные ENV
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range originalEnv {
			parts := splitEnv(e)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()

	f := func(price, devices, months, validHours uint16) bool {
		// Ограничиваем значения разумными диапазонами (избегаем 0)
		priceVal := int(price%10000) + 1      // 1-10000 рублей
		devicesVal := int(devices%20) + 1     // 1-20 устройств
		monthsVal := int(months%12) + 1       // 1-12 месяцев
		validHoursVal := int(validHours%168) + 1 // 1-168 часов (неделя)

		// Устанавливаем ENV переменные
		os.Setenv("WINBACK_PRICE", strconv.Itoa(priceVal))
		os.Setenv("WINBACK_DEVICES", strconv.Itoa(devicesVal))
		os.Setenv("WINBACK_MONTHS", strconv.Itoa(monthsVal))
		os.Setenv("WINBACK_VALID_HOURS", strconv.Itoa(validHoursVal))

		// Парсим конфигурацию напрямую через envIntDefault (как в InitConfig)
		parsedPrice := envIntDefault("WINBACK_PRICE", 100)
		parsedDevices := envIntDefault("WINBACK_DEVICES", 1)
		parsedMonths := envIntDefault("WINBACK_MONTHS", 1)
		parsedValidHours := envIntDefault("WINBACK_VALID_HOURS", 48)

		// Проверяем что значения совпадают с установленными (без конвертации)
		if parsedPrice != priceVal {
			t.Logf("WINBACK_PRICE: expected %d, got %d", priceVal, parsedPrice)
			return false
		}
		if parsedDevices != devicesVal {
			t.Logf("WINBACK_DEVICES: expected %d, got %d", devicesVal, parsedDevices)
			return false
		}
		if parsedMonths != monthsVal {
			t.Logf("WINBACK_MONTHS: expected %d, got %d", monthsVal, parsedMonths)
			return false
		}
		if parsedValidHours != validHoursVal {
			t.Logf("WINBACK_VALID_HOURS: expected %d, got %d", validHoursVal, parsedValidHours)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// TestWinbackConfigDefaults проверяет значения по умолчанию
func TestWinbackConfigDefaults(t *testing.T) {
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range originalEnv {
			parts := splitEnv(e)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()

	// Очищаем winback переменные
	os.Unsetenv("WINBACK_PRICE")
	os.Unsetenv("WINBACK_DEVICES")
	os.Unsetenv("WINBACK_MONTHS")
	os.Unsetenv("WINBACK_VALID_HOURS")

	// Проверяем значения по умолчанию
	if envIntDefault("WINBACK_PRICE", 100) != 100 {
		t.Error("Default WINBACK_PRICE should be 100")
	}
	if envIntDefault("WINBACK_DEVICES", 1) != 1 {
		t.Error("Default WINBACK_DEVICES should be 1")
	}
	if envIntDefault("WINBACK_MONTHS", 1) != 1 {
		t.Error("Default WINBACK_MONTHS should be 1")
	}
	if envIntDefault("WINBACK_VALID_HOURS", 48) != 48 {
		t.Error("Default WINBACK_VALID_HOURS should be 48")
	}
}
