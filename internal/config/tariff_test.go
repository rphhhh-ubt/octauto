package config

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/quick"
)

// **Feature: tariff-system, Property 1: Tariff Configuration Parsing**
// **Validates: Requirements 1.1, 1.2, 1.3, 1.4**
// *For any* valid ENV configuration with N enabled tariffs, parsing SHALL return
// exactly N tariffs with correct devices and prices values.

func TestParseTariffsProperty(t *testing.T) {
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

	f := func(
		name1Enabled bool, name1Devices uint8, name1Price1, name1Price3, name1Price6, name1Price12 uint16,
		name2Enabled bool, name2Devices uint8, name2Price1, name2Price3, name2Price6, name2Price12 uint16,
	) bool {
		// Очищаем все TARIFF_ переменные
		clearTariffEnv()

		// Ограничиваем значения разумными диапазонами
		d1 := int(name1Devices%20) + 1 // 1-20 устройств
		d2 := int(name2Devices%20) + 1
		p1_1 := int(name1Price1%10000) + 1 // 1-10000 цена
		p1_3 := int(name1Price3%10000) + 1
		p1_6 := int(name1Price6%10000) + 1
		p1_12 := int(name1Price12%10000) + 1
		p2_1 := int(name2Price1%10000) + 1
		p2_3 := int(name2Price3%10000) + 1
		p2_6 := int(name2Price6%10000) + 1
		p2_12 := int(name2Price12%10000) + 1

		expectedCount := 0

		// Устанавливаем тариф START
		if name1Enabled {
			os.Setenv("TARIFF_START_ENABLED", "true")
			os.Setenv("TARIFF_START_DEVICES", strconv.Itoa(d1))
			os.Setenv("TARIFF_START_PRICE_1", strconv.Itoa(p1_1))
			os.Setenv("TARIFF_START_PRICE_3", strconv.Itoa(p1_3))
			os.Setenv("TARIFF_START_PRICE_6", strconv.Itoa(p1_6))
			os.Setenv("TARIFF_START_PRICE_12", strconv.Itoa(p1_12))
			expectedCount++
		}

		// Устанавливаем тариф PRO
		if name2Enabled {
			os.Setenv("TARIFF_PRO_ENABLED", "true")
			os.Setenv("TARIFF_PRO_DEVICES", strconv.Itoa(d2))
			os.Setenv("TARIFF_PRO_PRICE_1", strconv.Itoa(p2_1))
			os.Setenv("TARIFF_PRO_PRICE_3", strconv.Itoa(p2_3))
			os.Setenv("TARIFF_PRO_PRICE_6", strconv.Itoa(p2_6))
			os.Setenv("TARIFF_PRO_PRICE_12", strconv.Itoa(p2_12))
			expectedCount++
		}

		// Парсим тарифы
		tariffs := parseTariffs()

		// Проверяем количество
		if len(tariffs) != expectedCount {
			t.Logf("Expected %d tariffs, got %d", expectedCount, len(tariffs))
			return false
		}

		// Проверяем корректность значений
		for _, tariff := range tariffs {
			switch tariff.Name {
			case "START":
				if tariff.Devices != d1 {
					t.Logf("START devices: expected %d, got %d", d1, tariff.Devices)
					return false
				}
				if tariff.Price1 != p1_1 || tariff.Price3 != p1_3 ||
					tariff.Price6 != p1_6 || tariff.Price12 != p1_12 {
					t.Logf("START prices mismatch")
					return false
				}
			case "PRO":
				if tariff.Devices != d2 {
					t.Logf("PRO devices: expected %d, got %d", d2, tariff.Devices)
					return false
				}
				if tariff.Price1 != p2_1 || tariff.Price3 != p2_3 ||
					tariff.Price6 != p2_6 || tariff.Price12 != p2_12 {
					t.Logf("PRO prices mismatch")
					return false
				}
			default:
				t.Logf("Unexpected tariff name: %s", tariff.Name)
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

// TestParseTariffsDisabledNotIncluded проверяет что отключённые тарифы не включаются
func TestParseTariffsDisabledNotIncluded(t *testing.T) {
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

	clearTariffEnv()

	// Тариф с ENABLED=false
	os.Setenv("TARIFF_TEST_ENABLED", "false")
	os.Setenv("TARIFF_TEST_DEVICES", "5")
	os.Setenv("TARIFF_TEST_PRICE_1", "100")
	os.Setenv("TARIFF_TEST_PRICE_3", "250")
	os.Setenv("TARIFF_TEST_PRICE_6", "450")
	os.Setenv("TARIFF_TEST_PRICE_12", "800")

	tariffs := parseTariffs()
	if len(tariffs) != 0 {
		t.Errorf("Expected 0 tariffs for disabled tariff, got %d", len(tariffs))
	}
}

// TestParseTariffsMissingPricesSkipped проверяет что тарифы без цен пропускаются
func TestParseTariffsMissingPricesSkipped(t *testing.T) {
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

	clearTariffEnv()

	// Тариф без PRICE_3
	os.Setenv("TARIFF_INCOMPLETE_ENABLED", "true")
	os.Setenv("TARIFF_INCOMPLETE_DEVICES", "5")
	os.Setenv("TARIFF_INCOMPLETE_PRICE_1", "100")
	// PRICE_3 отсутствует
	os.Setenv("TARIFF_INCOMPLETE_PRICE_6", "450")
	os.Setenv("TARIFF_INCOMPLETE_PRICE_12", "800")

	tariffs := parseTariffs()
	if len(tariffs) != 0 {
		t.Errorf("Expected 0 tariffs for incomplete tariff, got %d", len(tariffs))
	}
}

// TestStarsPricesDefaultToRegularPrices проверяет что цены в звёздах по умолчанию = обычным ценам
func TestStarsPricesDefaultToRegularPrices(t *testing.T) {
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

	clearTariffEnv()

	os.Setenv("TARIFF_BASIC_ENABLED", "true")
	os.Setenv("TARIFF_BASIC_DEVICES", "3")
	os.Setenv("TARIFF_BASIC_PRICE_1", "99")
	os.Setenv("TARIFF_BASIC_PRICE_3", "249")
	os.Setenv("TARIFF_BASIC_PRICE_6", "449")
	os.Setenv("TARIFF_BASIC_PRICE_12", "799")
	// Не устанавливаем STARS_PRICE_*

	tariffs := parseTariffs()
	if len(tariffs) != 1 {
		t.Fatalf("Expected 1 tariff, got %d", len(tariffs))
	}

	tariff := tariffs[0]
	if tariff.StarsPrice1 != 99 || tariff.StarsPrice3 != 249 ||
		tariff.StarsPrice6 != 449 || tariff.StarsPrice12 != 799 {
		t.Errorf("Stars prices should default to regular prices")
	}
}

// **Feature: tariff-system, Property 5: Tariff Button Text Contains Required Info**
// **Validates: Requirements 2.2**
// *For any* tariff, the generated button text SHALL contain the tariff name and device count.

func TestFormatButtonTextProperty(t *testing.T) {
	f := func(name string, devices uint8) bool {
		// Ограничиваем имя разумной длиной и убираем пустые строки
		if len(name) == 0 || len(name) > 50 {
			return true // Пропускаем невалидные входные данные
		}

		// Ограничиваем devices разумным диапазоном (1-255)
		d := int(devices)
		if d == 0 {
			d = 1
		}

		tariff := Tariff{
			Name:    name,
			Devices: d,
		}

		buttonText := tariff.FormatButtonText()

		// Проверяем что текст содержит имя тарифа
		if !strings.Contains(buttonText, name) {
			t.Logf("Button text does not contain tariff name. Text: %q, Name: %q", buttonText, name)
			return false
		}

		// Проверяем что текст содержит количество устройств как число
		devicesStr := strconv.Itoa(d)
		if !strings.Contains(buttonText, devicesStr) {
			t.Logf("Button text does not contain device count. Text: %q, Devices: %d", buttonText, d)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// TestFormatButtonTextExamples проверяет конкретные примеры
func TestFormatButtonTextExamples(t *testing.T) {
	tests := []struct {
		name     string
		tariff   Tariff
		wantName bool
		wantDev  bool
	}{
		{
			name:     "START tariff",
			tariff:   Tariff{Name: "START", Devices: 3},
			wantName: true,
			wantDev:  true,
		},
		{
			name:     "PRO tariff",
			tariff:   Tariff{Name: "PRO", Devices: 6},
			wantName: true,
			wantDev:  true,
		},
		{
			name:     "UNLIMITED tariff",
			tariff:   Tariff{Name: "UNLIMITED", Devices: 10},
			wantName: true,
			wantDev:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.tariff.FormatButtonText()

			if tt.wantName && !strings.Contains(result, tt.tariff.Name) {
				t.Errorf("FormatButtonText() = %q, should contain name %q", result, tt.tariff.Name)
			}

			devStr := strconv.Itoa(tt.tariff.Devices)
			if tt.wantDev && !strings.Contains(result, devStr) {
				t.Errorf("FormatButtonText() = %q, should contain devices %s", result, devStr)
			}
		})
	}
}

func clearTariffEnv() {
	for _, e := range os.Environ() {
		parts := splitEnv(e)
		if len(parts) == 2 && len(parts[0]) > 7 && parts[0][:7] == "TARIFF_" {
			os.Unsetenv(parts[0])
		}
	}
}

func splitEnv(env string) []string {
	for i := 0; i < len(env); i++ {
		if env[i] == '=' {
			return []string{env[:i], env[i+1:]}
		}
	}
	return []string{env}
}
