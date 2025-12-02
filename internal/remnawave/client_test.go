package remnawave

import (
	"testing"
	"testing/quick"
)

// **Feature: tariff-system, Property 2: Device Limit Resolution for Null**
// **Validates: Requirements 3.2**
// *For any* user with currentLimit == nil, ResolveDeviceLimit SHALL return tariffLimit (apply selected tariff).
func TestResolveDeviceLimit_NullCurrentLimit(t *testing.T) {
	f := func(tariffLimit int, allTariffLimits []int) bool {
		// Для любого tariffLimit и любого списка тарифов,
		// если currentLimit == nil, результат должен быть tariffLimit
		result := ResolveDeviceLimit(nil, tariffLimit, allTariffLimits)
		return result != nil && *result == tariffLimit
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 2 failed: %v", err)
	}
}

// **Feature: tariff-system, Property 3: Device Limit Resolution for Custom Limits**
// **Validates: Requirements 3.3**
// *For any* user with currentLimit not in tariff limits list, ResolveDeviceLimit SHALL return currentLimit unchanged.
func TestResolveDeviceLimit_CustomLimit(t *testing.T) {
	f := func(currentLimit int, tariffLimit int, allTariffLimits []int) bool {
		// Проверяем, что currentLimit НЕ в списке тарифов
		isInList := false
		for _, limit := range allTariffLimits {
			if currentLimit == limit {
				isInList = true
				break
			}
		}

		// Если currentLimit в списке - пропускаем этот тест-кейс (не наш случай)
		if isInList {
			return true
		}

		// Для кастомного лимита результат должен быть равен currentLimit
		result := ResolveDeviceLimit(&currentLimit, tariffLimit, allTariffLimits)
		return result != nil && *result == currentLimit
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 3 failed: %v", err)
	}
}

// **Feature: tariff-system, Property 4: Device Limit Resolution for Standard Limits**
// **Validates: Requirements 3.4, 3.5**
// *For any* user with currentLimit in tariff limits list, ResolveDeviceLimit SHALL return the selected tariff's limit.
func TestResolveDeviceLimit_StandardLimit(t *testing.T) {
	f := func(tariffLimit int, allTariffLimits []int) bool {
		// Нужен хотя бы один тариф в списке
		if len(allTariffLimits) == 0 {
			return true
		}

		// Берём первый лимит из списка как currentLimit (гарантированно стандартный)
		currentLimit := allTariffLimits[0]

		// Для стандартного лимита результат должен быть равен tariffLimit
		result := ResolveDeviceLimit(&currentLimit, tariffLimit, allTariffLimits)
		return result != nil && *result == tariffLimit
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 4 failed: %v", err)
	}
}
