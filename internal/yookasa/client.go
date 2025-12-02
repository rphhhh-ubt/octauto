package yookasa

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"remnawave-tg-shop-bot/internal/config"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type YookasaAPI interface {
	CreatePayment(ctx context.Context, request PaymentRequest, idempotencyKey string) (*Payment, error)
	GetPayment(ctx context.Context, paymentID uuid.UUID) (*Payment, error)
}

type Client struct {
	httpClient *http.Client
	baseURL    string
	authHeader string
}

func NewClient(baseURL, shopID, secretKey string) *Client {
	auth := fmt.Sprintf("%s:%s", shopID, secretKey)
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))

	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL:    baseURL,
		authHeader: fmt.Sprintf("Basic %s", encodedAuth),
	}
}

func (c *Client) CreateInvoice(ctx context.Context, amount int, month int, customerId int64, purchaseId int64) (*Payment, error) {
	return c.CreateInvoiceWithSave(ctx, amount, month, customerId, purchaseId, false, "", 0)
}

// CreateInvoiceWithSave создаёт платёж с опциональным сохранением способа оплаты для автопродления
// savePaymentMethod - если true, карта будет сохранена для рекуррентных платежей
// tariffName - название тарифа для сохранения в метаданных (для рекуррентных платежей)
// recurringAmount - сумма для автопродления (может отличаться от текущего платежа)
func (c *Client) CreateInvoiceWithSave(ctx context.Context, amount int, month int, customerId int64, purchaseId int64, savePaymentMethod bool, tariffName string, recurringAmount int) (*Payment, error) {
	rub := Amount{
		Value:    strconv.Itoa(amount),
		Currency: "RUB",
	}

	var monthString string
	switch month {
	case 1:
		monthString = "месяц"
	case 3, 4:
		monthString = "месяца"
	default:
		monthString = "месяцев"
	}

	description := fmt.Sprintf("Подписка на %d %s", month, monthString)
	receipt := &Receipt{
		Customer: &Customer{
			Email: config.YookasaEmail(),
		},
		Items: []Item{
			{
				VatCode:        1,
				Quantity:       "1",
				Description:    description,
				Amount:         rub,
				PaymentSubject: "payment",
				PaymentMode:    "full_payment",
			},
		},
	}

	metaData := map[string]any{
		"customerId": customerId,
		"purchaseId": purchaseId,
		"username":   ctx.Value("username"),
	}

	// Добавляем данные для рекуррентных платежей если включено сохранение
	if savePaymentMethod {
		metaData["recurring_enabled"] = true
		metaData["recurring_tariff_name"] = tariffName
		metaData["recurring_months"] = month
		metaData["recurring_amount"] = recurringAmount
	}

	paymentRequest := NewPaymentRequest(
		rub,
		config.BotURL(),
		description,
		receipt,
		metaData,
	)

	// Устанавливаем флаг сохранения способа оплаты
	paymentRequest.SavePaymentMethod = savePaymentMethod

	idempotencyKey := uuid.New().String()

	payment, err := c.CreatePayment(ctx, paymentRequest, idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create payment: %w", err)
	}

	return payment, nil
}

// CreateRecurringPayment создаёт автоплатёж по сохранённому способу оплаты (payment_method_id)
// Не требует подтверждения пользователя - деньги списываются автоматически
func (c *Client) CreateRecurringPayment(ctx context.Context, paymentMethodID uuid.UUID, amount int, months int, customerId int64, description string) (*Payment, error) {
	rub := Amount{
		Value:    strconv.Itoa(amount),
		Currency: "RUB",
	}

	receipt := &Receipt{
		Customer: &Customer{
			Email: config.YookasaEmail(),
		},
		Items: []Item{
			{
				VatCode:        1,
				Quantity:       "1",
				Description:    description,
				Amount:         rub,
				PaymentSubject: "payment",
				PaymentMode:    "full_payment",
			},
		},
	}

	metaData := map[string]any{
		"customerId":        customerId,
		"recurring_payment": true,
		"months":            months,
	}

	// Для рекуррентного платежа не нужен redirect - используем payment_method_id
	paymentRequest := PaymentRequest{
		Amount:          rub,
		Capture:         true,
		Description:     description,
		PaymentMethodID: &paymentMethodID,
		Receipt:         receipt,
		Metadata:        metaData,
	}

	idempotencyKey := uuid.New().String()

	payment, err := c.CreatePayment(ctx, paymentRequest, idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create recurring payment: %w", err)
	}

	return payment, nil
}

func (c *Client) CreatePayment(ctx context.Context, request PaymentRequest, idempotencyKey string) (*Payment, error) {
	paymentURL := fmt.Sprintf("%s/payments", c.baseURL)

	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payment request: %w", err)
	}

	log.Printf("YooKassa CreatePayment request: %s", string(reqBody))

	req, err := http.NewRequestWithContext(ctx, "POST", paymentURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Idempotence-Key", idempotencyKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error while reading invoice resp: %w", err)
		}
		return nil, fmt.Errorf("API return error. Status: %d, Body: %s", resp.StatusCode, string(body))
	}

	var payment Payment
	if err := json.NewDecoder(resp.Body).Decode(&payment); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &payment, nil
}

func (c *Client) GetPayment(ctx context.Context, paymentID uuid.UUID) (*Payment, error) {
	paymentURL := fmt.Sprintf("%s/payments/%s", c.baseURL, paymentID)

	var payment *Payment

	maxRetries := 5
	baseDelay := time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "GET", paymentURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", c.authHeader)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			payment = new(Payment)
			if err := json.NewDecoder(resp.Body).Decode(payment); err != nil {
				return nil, fmt.Errorf("failed to decode response: %w", err)
			}
			return payment, nil
		}

		// Retry on server errors: 429 (rate limit), 500 (internal), 502 (bad gateway), 503 (unavailable), 504 (timeout)
		if resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusInternalServerError ||
			resp.StatusCode == http.StatusBadGateway ||
			resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode == http.StatusGatewayTimeout {
			retryDelay := baseDelay * time.Duration(1<<attempt)
			log.Printf("Received %d from YooKassa. Retrying in %v... (attempt %d/%d)", resp.StatusCode, retryDelay, attempt+1, maxRetries)
			time.Sleep(retryDelay)
			continue
		}

		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil, fmt.Errorf("exceeded maximum retries due to server errors")
}
