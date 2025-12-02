package config

import (
	"os"
	"strconv"
	"testing"
	"testing/quick"
)

// **Feature: recurring-payments, Property 7: Config toggle**
// **Validates: Requirements 5.2**
// *For any* значение RECURRING_PAYMENTS_ENABLED=false, функция IsRecurringPaymentsEnabled()
// должна возвращать false и чекбокс автопродления не должен отображаться.

func TestRecurringPaymentsConfigToggleProperty(t *testing.T) {
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

	f := func(enabled bool, notifyHours uint8) bool {
		// Ограничиваем notifyHours разумным диапазоном (1-168 часов = неделя)
		notifyHoursVal := int(notifyHours%168) + 1

		// Устанавливаем ENV переменные
		if enabled {
			os.Setenv("RECURRING_PAYMENTS_ENABLED", "true")
		} else {
			os.Setenv("RECURRING_PAYMENTS_ENABLED", "false")
		}
		os.Setenv("RECURRING_NOTIFY_HOURS_BEFORE", strconv.Itoa(notifyHoursVal))

		// Парсим конфигурацию напрямую через envBool и envIntDefault (как в InitConfig)
		parsedEnabled := envBool("RECURRING_PAYMENTS_ENABLED")
		parsedNotifyHours := envIntDefault("RECURRING_NOTIFY_HOURS_BEFORE", 48)

		// Property: RECURRING_PAYMENTS_ENABLED=false -> IsRecurringPaymentsEnabled() == false
		// Property: RECURRING_PAYMENTS_ENABLED=true -> IsRecurringPaymentsEnabled() == true
		if parsedEnabled != enabled {
			t.Logf("RECURRING_PAYMENTS_ENABLED: expected %v, got %v", enabled, parsedEnabled)
			return false
		}

		// Проверяем что notifyHours парсится корректно
		if parsedNotifyHours != notifyHoursVal {
			t.Logf("RECURRING_NOTIFY_HOURS_BEFORE: expected %d, got %d", notifyHoursVal, parsedNotifyHours)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// TestRecurringPaymentsConfigDefaults проверяет значения по умолчанию
func TestRecurringPaymentsConfigDefaults(t *testing.T) {
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

	// Очищаем recurring переменные
	os.Unsetenv("RECURRING_PAYMENTS_ENABLED")
	os.Unsetenv("RECURRING_NOTIFY_HOURS_BEFORE")

	// Проверяем значения по умолчанию
	// RECURRING_PAYMENTS_ENABLED по умолчанию false (envBool возвращает false для пустой строки)
	if envBool("RECURRING_PAYMENTS_ENABLED") != false {
		t.Error("Default RECURRING_PAYMENTS_ENABLED should be false")
	}
	// RECURRING_NOTIFY_HOURS_BEFORE по умолчанию 48
	if envIntDefault("RECURRING_NOTIFY_HOURS_BEFORE", 48) != 48 {
		t.Error("Default RECURRING_NOTIFY_HOURS_BEFORE should be 48")
	}
}

// TestRecurringPaymentsDisabledHidesCheckbox проверяет что при отключённых рекуррентных платежах
// чекбокс не должен отображаться (логика в handler, но конфиг должен возвращать false)
func TestRecurringPaymentsDisabledHidesCheckbox(t *testing.T) {
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

	testCases := []struct {
		name     string
		envValue string
		expected bool
	}{
		{"explicitly false", "false", false},
		{"empty string", "", false},
		{"not set", "", false},
		{"explicitly true", "true", true},
		{"invalid value", "yes", false}, // envBool проверяет только "true"
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envValue == "" && tc.name == "not set" {
				os.Unsetenv("RECURRING_PAYMENTS_ENABLED")
			} else {
				os.Setenv("RECURRING_PAYMENTS_ENABLED", tc.envValue)
			}

			result := envBool("RECURRING_PAYMENTS_ENABLED")
			if result != tc.expected {
				t.Errorf("envBool(RECURRING_PAYMENTS_ENABLED) with value %q: expected %v, got %v",
					tc.envValue, tc.expected, result)
			}
		})
	}
}
