package config

import (
	"os"
	"testing"
	"testing/quick"
)

// **Feature: promo-tariff-discount, Property 1: Config Feature Flag**
// **Validates: Requirements 1.1**
// *For any* ENV configuration, when `PROMO_TARIFF_CODES_ENABLED` is set to "true",
// `IsPromoTariffCodesEnabled()` should return true, otherwise false.

func TestPromoTariffCodesConfigToggleProperty(t *testing.T) {
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

	f := func(enabled bool) bool {
		// Устанавливаем ENV переменную
		if enabled {
			os.Setenv("PROMO_TARIFF_CODES_ENABLED", "true")
		} else {
			os.Setenv("PROMO_TARIFF_CODES_ENABLED", "false")
		}

		// Парсим конфигурацию через envBool (как в InitConfig)
		parsedEnabled := envBool("PROMO_TARIFF_CODES_ENABLED")

		// Property: PROMO_TARIFF_CODES_ENABLED=true -> true, иначе false
		if parsedEnabled != enabled {
			t.Logf("PROMO_TARIFF_CODES_ENABLED: expected %v, got %v", enabled, parsedEnabled)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// TestPromoTariffCodesConfigDefaults проверяет значения по умолчанию
func TestPromoTariffCodesConfigDefaults(t *testing.T) {
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

	// Очищаем переменную
	os.Unsetenv("PROMO_TARIFF_CODES_ENABLED")

	// По умолчанию false (envBool возвращает false для пустой строки)
	if envBool("PROMO_TARIFF_CODES_ENABLED") != false {
		t.Error("Default PROMO_TARIFF_CODES_ENABLED should be false")
	}
}

// TestPromoTariffCodesEnvValues проверяет различные значения ENV
func TestPromoTariffCodesEnvValues(t *testing.T) {
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
		{"explicitly true", "true", true},
		{"invalid value yes", "yes", false},
		{"invalid value 1", "1", false},
		{"invalid value TRUE", "TRUE", false}, // envBool проверяет только "true"
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envValue == "" {
				os.Unsetenv("PROMO_TARIFF_CODES_ENABLED")
			} else {
				os.Setenv("PROMO_TARIFF_CODES_ENABLED", tc.envValue)
			}

			result := envBool("PROMO_TARIFF_CODES_ENABLED")
			if result != tc.expected {
				t.Errorf("envBool(PROMO_TARIFF_CODES_ENABLED) with value %q: expected %v, got %v",
					tc.envValue, tc.expected, result)
			}
		})
	}
}
