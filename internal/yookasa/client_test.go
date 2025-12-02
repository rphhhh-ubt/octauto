package yookasa

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/quick"

	"github.com/google/uuid"
)

// **Feature: recurring-payments, Property 1: Save payment method flag propagation**
// **Validates: Requirements 1.2**
// *For any* платёж с включённым автопродлением, запрос к ЮKassa должен содержать save_payment_method=true

func TestSavePaymentMethodFlagPropagation(t *testing.T) {
	f := func(
		amount uint16,
		month uint8,
		customerId int64,
		purchaseId int64,
		savePaymentMethod bool,
		tariffNameBytes [10]byte,
		recurringAmount uint16,
	) bool {
		// Ограничиваем входные данные разумными значениями
		amt := int(amount%10000) + 1       // 1-10000 рублей
		m := int(month%12) + 1             // 1-12 месяцев
		recAmt := int(recurringAmount%10000) + 1

		// Генерируем tariffName из байтов (только ASCII буквы)
		tariffName := ""
		for _, b := range tariffNameBytes {
			if b >= 'A' && b <= 'Z' {
				tariffName += string(b)
			} else if b >= 'a' && b <= 'z' {
				tariffName += string(b)
			}
		}
		if tariffName == "" {
			tariffName = "DEFAULT"
		}

		// Переменная для захвата запроса
		var capturedRequest PaymentRequest

		// Создаём тестовый сервер
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Декодируем запрос
			if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
				t.Logf("Failed to decode request: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}

			// Возвращаем успешный ответ
			response := Payment{
				ID:     uuid.New(),
				Status: "pending",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		// Создаём клиент с тестовым сервером
		client := NewClient(server.URL, "test-shop-id", "test-secret-key")

		// Вызываем CreateInvoiceWithSave
		ctx := context.WithValue(context.Background(), "username", "testuser")
		_, err := client.CreateInvoiceWithSave(ctx, amt, m, customerId, purchaseId, savePaymentMethod, tariffName, recAmt)
		if err != nil {
			t.Logf("CreateInvoiceWithSave failed: %v", err)
			return false
		}

		// PROPERTY: save_payment_method в запросе должен соответствовать входному параметру
		if capturedRequest.SavePaymentMethod != savePaymentMethod {
			t.Logf("SavePaymentMethod mismatch: expected %v, got %v", savePaymentMethod, capturedRequest.SavePaymentMethod)
			return false
		}

		// Дополнительная проверка: если savePaymentMethod=true, метаданные должны содержать recurring_enabled=true
		if savePaymentMethod {
			if capturedRequest.Metadata == nil {
				t.Logf("Metadata is nil when savePaymentMethod=true")
				return false
			}
			if enabled, ok := capturedRequest.Metadata["recurring_enabled"]; !ok || enabled != true {
				t.Logf("recurring_enabled not set correctly in metadata")
				return false
			}
			if tariff, ok := capturedRequest.Metadata["recurring_tariff_name"]; !ok || tariff != tariffName {
				t.Logf("recurring_tariff_name mismatch: expected %s, got %v", tariffName, tariff)
				return false
			}
			// JSON декодирует числа как float64
			if months, ok := capturedRequest.Metadata["recurring_months"]; !ok || int(months.(float64)) != m {
				t.Logf("recurring_months mismatch: expected %d, got %v", m, months)
				return false
			}
			if recAmount, ok := capturedRequest.Metadata["recurring_amount"]; !ok || int(recAmount.(float64)) != recAmt {
				t.Logf("recurring_amount mismatch: expected %d, got %v", recAmt, recAmount)
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

// **Feature: recurring-payments, Property 3: Recurring payment execution**
// **Validates: Requirements 2.2**
// *For any* пользователь с recurring_enabled=true и валидным payment_method_id,
// при получении webhook об истечении подписки система должна создать автоплатёж

func TestRecurringPaymentExecution(t *testing.T) {
	f := func(
		paymentMethodIDBytes [16]byte,
		amount uint16,
		months uint8,
		customerIdRaw uint32, // Используем uint32 для безопасного диапазона JSON float64
		descriptionBytes [20]byte,
	) bool {
		// Ограничиваем входные данные разумными значениями
		amt := int(amount%10000) + 1 // 1-10000 рублей
		m := int(months%12) + 1      // 1-12 месяцев
		customerId := int64(customerIdRaw) // Telegram ID обычно положительные числа

		// Генерируем UUID из байтов
		paymentMethodID, err := uuid.FromBytes(paymentMethodIDBytes[:])
		if err != nil {
			// Если не удалось создать UUID, используем новый
			paymentMethodID = uuid.New()
		}

		// Генерируем description из байтов (только ASCII буквы и пробелы)
		description := ""
		for _, b := range descriptionBytes {
			if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == ' ' {
				description += string(b)
			}
		}
		if description == "" {
			description = "Autopayment"
		}

		// Переменная для захвата запроса
		var capturedRequest PaymentRequest

		// Создаём тестовый сервер
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Декодируем запрос
			if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
				t.Logf("Failed to decode request: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}

			// Возвращаем успешный ответ
			response := Payment{
				ID:     uuid.New(),
				Status: "succeeded",
				Paid:   true,
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		// Создаём клиент с тестовым сервером
		client := NewClient(server.URL, "test-shop-id", "test-secret-key")

		// Вызываем CreateRecurringPayment
		ctx := context.Background()
		_, err = client.CreateRecurringPayment(ctx, paymentMethodID, amt, m, customerId, description)
		if err != nil {
			t.Logf("CreateRecurringPayment failed: %v", err)
			return false
		}

		// PROPERTY 1: payment_method_id должен быть установлен в запросе
		if capturedRequest.PaymentMethodID == nil {
			t.Logf("PaymentMethodID is nil in request")
			return false
		}

		// PROPERTY 2: payment_method_id должен соответствовать входному параметру
		if *capturedRequest.PaymentMethodID != paymentMethodID {
			t.Logf("PaymentMethodID mismatch: expected %s, got %s", paymentMethodID, *capturedRequest.PaymentMethodID)
			return false
		}

		// PROPERTY 3: Confirmation (redirect) НЕ должен быть установлен для рекуррентного платежа
		// (рекуррентный платёж не требует подтверждения пользователя)
		if capturedRequest.Confirmation != nil && (capturedRequest.Confirmation.Type != "" || capturedRequest.Confirmation.ReturnURL != "") {
			t.Logf("Confirmation should be empty for recurring payment, got: %+v", capturedRequest.Confirmation)
			return false
		}

		// PROPERTY 4: Метаданные должны содержать recurring_payment=true
		if capturedRequest.Metadata == nil {
			t.Logf("Metadata is nil")
			return false
		}
		if recurring, ok := capturedRequest.Metadata["recurring_payment"]; !ok || recurring != true {
			t.Logf("recurring_payment not set correctly in metadata")
			return false
		}

		// PROPERTY 5: Метаданные должны содержать customerId
		// JSON декодирует числа как float64
		if cid, ok := capturedRequest.Metadata["customerId"]; !ok || int64(cid.(float64)) != customerId {
			t.Logf("customerId mismatch in metadata: expected %d, got %v", customerId, cid)
			return false
		}

		// PROPERTY 6: Метаданные должны содержать months
		// JSON декодирует числа как float64
		if monthsVal, ok := capturedRequest.Metadata["months"]; !ok || int(monthsVal.(float64)) != m {
			t.Logf("months mismatch in metadata: expected %d, got %v", m, monthsVal)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// TestSavePaymentMethodFlagExamples - примеры для конкретных случаев
func TestSavePaymentMethodFlagExamples(t *testing.T) {
	tests := []struct {
		name              string
		savePaymentMethod bool
		wantFlag          bool
		wantMetadata      bool
	}{
		{
			name:              "recurring enabled",
			savePaymentMethod: true,
			wantFlag:          true,
			wantMetadata:      true,
		},
		{
			name:              "recurring disabled",
			savePaymentMethod: false,
			wantFlag:          false,
			wantMetadata:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedRequest PaymentRequest

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewDecoder(r.Body).Decode(&capturedRequest)
				response := Payment{ID: uuid.New(), Status: "pending"}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			client := NewClient(server.URL, "shop", "secret")
			ctx := context.WithValue(context.Background(), "username", "user")

			_, err := client.CreateInvoiceWithSave(ctx, 1000, 1, 123, 456, tt.savePaymentMethod, "START", 1000)
			if err != nil {
				t.Fatalf("CreateInvoiceWithSave failed: %v", err)
			}

			if capturedRequest.SavePaymentMethod != tt.wantFlag {
				t.Errorf("SavePaymentMethod = %v, want %v", capturedRequest.SavePaymentMethod, tt.wantFlag)
			}

			if tt.wantMetadata {
				if capturedRequest.Metadata["recurring_enabled"] != true {
					t.Errorf("recurring_enabled not set in metadata")
				}
			}
		})
	}
}
