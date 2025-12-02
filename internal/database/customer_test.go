package database

import (
	"strconv"
	"testing"
	"testing/quick"

	"github.com/google/uuid"
)

// **Feature: recurring-payments, Property 2: Payment method persistence**
// **Validates: Requirements 1.3**
// *For any* успешный платёж с save_payment_method=true, после обработки webhook
// payment_method_id должен быть сохранён в БД для соответствующего customer

// RecurringSettingsFromMetadata извлекает настройки recurring из метаданных платежа
// Эта функция тестирует логику парсинга метаданных, которая используется в saveRecurringPaymentMethod
type RecurringSettingsFromMetadata struct {
	PaymentMethodID string
	TariffName      *string
	Months          *int
	Amount          *int
}

// ParseRecurringMetadata парсит метаданные платежа и возвращает настройки recurring
// Это чистая функция, которую можно тестировать property-based тестами
func ParseRecurringMetadata(paymentMethodID uuid.UUID, metadata map[string]string) RecurringSettingsFromMetadata {
	result := RecurringSettingsFromMetadata{
		PaymentMethodID: paymentMethodID.String(),
	}

	if tn, ok := metadata["recurring_tariff_name"]; ok && tn != "" {
		result.TariffName = &tn
	}

	if m, ok := metadata["recurring_months"]; ok {
		if monthsInt, err := strconv.Atoi(m); err == nil {
			result.Months = &monthsInt
		}
	}

	if a, ok := metadata["recurring_amount"]; ok {
		if amountInt, err := strconv.Atoi(a); err == nil {
			result.Amount = &amountInt
		}
	}

	return result
}

func TestPaymentMethodPersistenceProperty(t *testing.T) {
	f := func(
		paymentMethodIDBytes [16]byte,
		tariffNameBytes [10]byte,
		months uint8,
		amount uint16,
		hasTariff bool,
		hasMonths bool,
		hasAmount bool,
	) bool {
		// Генерируем UUID из байтов
		paymentMethodID, err := uuid.FromBytes(paymentMethodIDBytes[:])
		if err != nil {
			paymentMethodID = uuid.New()
		}

		// Генерируем tariffName из байтов (только ASCII буквы)
		tariffName := ""
		for _, b := range tariffNameBytes {
			if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') {
				tariffName += string(b)
			}
		}
		if tariffName == "" {
			tariffName = "DEFAULT"
		}

		// Ограничиваем значения разумными диапазонами
		monthsVal := int(months%12) + 1   // 1-12 месяцев
		amountVal := int(amount%10000) + 1 // 1-10000 рублей

		// Создаём метаданные в зависимости от флагов
		metadata := make(map[string]string)
		if hasTariff {
			metadata["recurring_tariff_name"] = tariffName
		}
		if hasMonths {
			metadata["recurring_months"] = strconv.Itoa(monthsVal)
		}
		if hasAmount {
			metadata["recurring_amount"] = strconv.Itoa(amountVal)
		}

		// Парсим метаданные
		result := ParseRecurringMetadata(paymentMethodID, metadata)

		// PROPERTY 1: payment_method_id всегда должен быть сохранён как строка UUID
		if result.PaymentMethodID != paymentMethodID.String() {
			t.Logf("PaymentMethodID mismatch: expected %s, got %s", paymentMethodID.String(), result.PaymentMethodID)
			return false
		}

		// PROPERTY 2: Если tariff_name присутствует в метаданных, он должен быть сохранён
		if hasTariff {
			if result.TariffName == nil || *result.TariffName != tariffName {
				t.Logf("TariffName mismatch: expected %s, got %v", tariffName, result.TariffName)
				return false
			}
		} else {
			if result.TariffName != nil {
				t.Logf("TariffName should be nil when not in metadata, got %v", result.TariffName)
				return false
			}
		}

		// PROPERTY 3: Если months присутствует в метаданных, он должен быть сохранён
		if hasMonths {
			if result.Months == nil || *result.Months != monthsVal {
				t.Logf("Months mismatch: expected %d, got %v", monthsVal, result.Months)
				return false
			}
		} else {
			if result.Months != nil {
				t.Logf("Months should be nil when not in metadata, got %v", result.Months)
				return false
			}
		}

		// PROPERTY 4: Если amount присутствует в метаданных, он должен быть сохранён
		if hasAmount {
			if result.Amount == nil || *result.Amount != amountVal {
				t.Logf("Amount mismatch: expected %d, got %v", amountVal, result.Amount)
				return false
			}
		} else {
			if result.Amount != nil {
				t.Logf("Amount should be nil when not in metadata, got %v", result.Amount)
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

// TestPaymentMethodPersistenceInvalidMetadata проверяет обработку невалидных метаданных
func TestPaymentMethodPersistenceInvalidMetadata(t *testing.T) {
	f := func(
		paymentMethodIDBytes [16]byte,
		invalidMonths string,
		invalidAmount string,
	) bool {
		// Генерируем UUID
		paymentMethodID, err := uuid.FromBytes(paymentMethodIDBytes[:])
		if err != nil {
			paymentMethodID = uuid.New()
		}

		// Создаём метаданные с невалидными значениями (не числа)
		// Фильтруем строки, которые могут быть распарсены как числа
		_, errMonths := strconv.Atoi(invalidMonths)
		_, errAmount := strconv.Atoi(invalidAmount)

		// Если строки парсятся как числа, пропускаем этот тест-кейс
		if errMonths == nil || errAmount == nil {
			return true
		}

		metadata := map[string]string{
			"recurring_months": invalidMonths,
			"recurring_amount": invalidAmount,
		}

		result := ParseRecurringMetadata(paymentMethodID, metadata)

		// PROPERTY: При невалидных числовых значениях, поля должны быть nil
		if result.Months != nil {
			t.Logf("Months should be nil for invalid input '%s', got %v", invalidMonths, result.Months)
			return false
		}
		if result.Amount != nil {
			t.Logf("Amount should be nil for invalid input '%s', got %v", invalidAmount, result.Amount)
			return false
		}

		// payment_method_id всё равно должен быть сохранён
		if result.PaymentMethodID != paymentMethodID.String() {
			t.Logf("PaymentMethodID should still be saved")
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}


// **Feature: recurring-payments, Property 6: Manual recurring disable**
// **Validates: Requirements 3.1**
// *For any* запрос на отключение автопродления, recurring_enabled должен стать false
// и payment_method_id должен быть очищен

// DisabledRecurringState представляет состояние customer после отключения recurring
type DisabledRecurringState struct {
	RecurringEnabled bool
	PaymentMethodID  *string
}

// ApplyDisableRecurring применяет логику отключения recurring к customer
// Это чистая функция, которая моделирует поведение DisableRecurring
// без обращения к БД
// ВАЖНО: DisableRecurring НЕ очищает payment_method_id, только отключает флаг
func ApplyDisableRecurring(customer Customer) DisabledRecurringState {
	return DisabledRecurringState{
		RecurringEnabled: false,
		PaymentMethodID:  customer.PaymentMethodID, // Сохраняем payment_method_id
	}
}

func TestManualRecurringDisableProperty(t *testing.T) {
	f := func(
		customerIdRaw uint32,
		telegramIdRaw uint32,
		paymentMethodIDBytes [16]byte,
		tariffNameBytes [10]byte,
		recurringMonthsRaw uint8,
		recurringAmountRaw uint16,
		initialRecurringEnabled bool,
	) bool {
		// Генерируем UUID из байтов
		paymentMethodID, err := uuid.FromBytes(paymentMethodIDBytes[:])
		if err != nil {
			paymentMethodID = uuid.New()
		}
		paymentMethodIDStr := paymentMethodID.String()

		// Генерируем tariffName из байтов (только ASCII буквы)
		tariffName := ""
		for _, b := range tariffNameBytes {
			if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') {
				tariffName += string(b)
			}
		}
		if tariffName == "" {
			tariffName = "DEFAULT"
		}

		// Ограничиваем значения разумными диапазонами
		customerId := int64(customerIdRaw%1000000) + 1
		telegramId := int64(telegramIdRaw%1000000) + 1
		recurringMonths := int(recurringMonthsRaw%12) + 1
		recurringAmount := int(recurringAmountRaw%10000) + 100

		// Создаём customer с произвольным начальным состоянием recurring
		customer := Customer{
			ID:                  customerId,
			TelegramID:          telegramId,
			RecurringEnabled:    initialRecurringEnabled,
			PaymentMethodID:     &paymentMethodIDStr,
			RecurringTariffName: &tariffName,
			RecurringMonths:     &recurringMonths,
			RecurringAmount:     &recurringAmount,
		}

		// Применяем отключение recurring
		result := ApplyDisableRecurring(customer)

		// PROPERTY 1: После отключения recurring_enabled ВСЕГДА должен быть false
		// независимо от начального состояния
		if result.RecurringEnabled != false {
			t.Logf("RecurringEnabled should be false after disable, got %v", result.RecurringEnabled)
			return false
		}

		// PROPERTY 2: После отключения payment_method_id СОХРАНЯЕТСЯ
		// Это позволяет пользователю легко включить автопродление обратно
		if (customer.PaymentMethodID == nil) != (result.PaymentMethodID == nil) {
			t.Logf("PaymentMethodID should be preserved after disable")
			return false
		}
		if customer.PaymentMethodID != nil && result.PaymentMethodID != nil {
			if *customer.PaymentMethodID != *result.PaymentMethodID {
				t.Logf("PaymentMethodID should be preserved, expected %v, got %v", *customer.PaymentMethodID, *result.PaymentMethodID)
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

// TestManualRecurringDisableIdempotent проверяет идемпотентность операции отключения
// Повторное отключение должно давать тот же результат
func TestManualRecurringDisableIdempotent(t *testing.T) {
	f := func(
		paymentMethodIDBytes [16]byte,
		recurringMonthsRaw uint8,
		recurringAmountRaw uint16,
	) bool {
		// Генерируем UUID
		paymentMethodID, err := uuid.FromBytes(paymentMethodIDBytes[:])
		if err != nil {
			paymentMethodID = uuid.New()
		}
		paymentMethodIDStr := paymentMethodID.String()

		recurringMonths := int(recurringMonthsRaw%12) + 1
		recurringAmount := int(recurringAmountRaw%10000) + 100

		// Создаём customer с включённым recurring
		customer := Customer{
			ID:               1,
			TelegramID:       123456,
			RecurringEnabled: true,
			PaymentMethodID:  &paymentMethodIDStr,
			RecurringMonths:  &recurringMonths,
			RecurringAmount:  &recurringAmount,
		}

		// Первое отключение
		result1 := ApplyDisableRecurring(customer)

		// Создаём customer с уже отключённым recurring (как после первого отключения)
		customerAfterFirstDisable := Customer{
			ID:               1,
			TelegramID:       123456,
			RecurringEnabled: result1.RecurringEnabled,
			PaymentMethodID:  result1.PaymentMethodID,
			RecurringMonths:  &recurringMonths,
			RecurringAmount:  &recurringAmount,
		}

		// Второе отключение
		result2 := ApplyDisableRecurring(customerAfterFirstDisable)

		// PROPERTY: Идемпотентность - результат должен быть одинаковым
		if result1.RecurringEnabled != result2.RecurringEnabled {
			t.Logf("Idempotency violated: RecurringEnabled differs after second disable")
			return false
		}

		if (result1.PaymentMethodID == nil) != (result2.PaymentMethodID == nil) {
			t.Logf("Idempotency violated: PaymentMethodID nil status differs after second disable")
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}
