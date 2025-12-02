package promo

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

var promoTariffCodeRegex = regexp.MustCompile(`^[A-Z0-9_-]{3,50}$`)

// TariffApplyResult результат применения промокода на тариф
type TariffApplyResult struct {
	Success      bool
	ErrorKey     string     // translation key for error message
	OfferExpires *time.Time // когда истекает предложение
}

// TariffService сервис для работы с промокодами на тариф
type TariffService struct {
	promoTariffRepo *database.PromoTariffRepository
	customerRepo    *database.CustomerRepository
}

// NewTariffService создаёт новый сервис промокодов на тариф
func NewTariffService(
	promoTariffRepo *database.PromoTariffRepository,
	customerRepo *database.CustomerRepository,
) *TariffService {
	return &TariffService{
		promoTariffRepo: promoTariffRepo,
		customerRepo:    customerRepo,
	}
}

// ValidationError represents a validation error with a key
type ValidationError struct {
	Key string
}

func (e *ValidationError) Error() string {
	return e.Key
}

// ValidatePromoTariffCode проверяет валидность данных для создания промокода
// Returns error key if validation fails, empty string if valid
func ValidatePromoTariffCode(code string, price, devices, months, maxActivations int) string {
	code = strings.ToUpper(strings.TrimSpace(code))

	if code == "" {
		return "promo_tariff_code_empty"
	}
	if !promoTariffCodeRegex.MatchString(code) {
		return "promo_tariff_invalid_format"
	}
	if price <= 0 {
		return "promo_tariff_invalid_price"
	}
	if devices <= 0 {
		return "promo_tariff_invalid_devices"
	}
	if months <= 0 {
		return "promo_tariff_invalid_months"
	}
	if maxActivations <= 0 {
		return "promo_tariff_invalid_max_activations"
	}
	return ""
}

// ApplyPromoTariffCode применяет промокод на тариф для пользователя
// Сохраняет предложение в customer и возвращает результат
func (s *TariffService) ApplyPromoTariffCode(ctx context.Context, customerID int64, code string) *TariffApplyResult {
	code = strings.ToUpper(strings.TrimSpace(code))

	// Validate format
	if !promoTariffCodeRegex.MatchString(code) {
		return &TariffApplyResult{Success: false, ErrorKey: "promo_tariff_invalid_format"}
	}

	// Find promo tariff code
	promo, err := s.promoTariffRepo.FindByCode(ctx, code)
	if err != nil {
		slog.Error("Error finding promo tariff code", "code", code, "error", err)
		return &TariffApplyResult{Success: false, ErrorKey: "promo_tariff_error"}
	}
	if promo == nil {
		return &TariffApplyResult{Success: false, ErrorKey: "promo_tariff_not_found"}
	}

	// Check if active
	if !promo.IsActive {
		return &TariffApplyResult{Success: false, ErrorKey: "promo_tariff_inactive"}
	}

	// Check expiration (valid_until - дата истечения самого промокода)
	if promo.ValidUntil != nil && time.Now().After(*promo.ValidUntil) {
		return &TariffApplyResult{Success: false, ErrorKey: "promo_tariff_expired"}
	}

	// Check activation limit
	if promo.CurrentActivations >= promo.MaxActivations {
		return &TariffApplyResult{Success: false, ErrorKey: "promo_tariff_limit_reached"}
	}

	// Check if already used by this customer
	used, err := s.promoTariffRepo.IsUsedByCustomer(ctx, promo.ID, customerID)
	if err != nil {
		slog.Error("Error checking promo tariff usage", "promoID", promo.ID, "customerID", customerID, "error", err)
		return &TariffApplyResult{Success: false, ErrorKey: "promo_tariff_error"}
	}
	if used {
		return &TariffApplyResult{Success: false, ErrorKey: "promo_tariff_already_used"}
	}

	// Calculate offer expiration
	offerExpires := time.Now().Add(time.Duration(promo.ValidHours) * time.Hour)

	// Save offer to customer
	if err := s.customerRepo.UpdatePromoOffer(ctx, customerID, promo.Price, promo.Devices, promo.Months, offerExpires, promo.ID); err != nil {
		slog.Error("Error saving promo offer to customer", "customerID", customerID, "error", err)
		return &TariffApplyResult{Success: false, ErrorKey: "promo_tariff_error"}
	}

	// Record activation
	if err := s.promoTariffRepo.RecordActivation(ctx, promo.ID, customerID); err != nil {
		slog.Error("Error recording promo tariff activation", "promoID", promo.ID, "customerID", customerID, "error", err)
		// Don't fail - offer already saved
	}

	// Increment counter
	if err := s.promoTariffRepo.IncrementActivations(ctx, promo.ID); err != nil {
		slog.Error("Error incrementing promo tariff activations", "promoID", promo.ID, "error", err)
	}

	slog.Info("Promo tariff code applied",
		"code", code,
		"customerID", customerID,
		"price", promo.Price,
		"devices", promo.Devices,
		"months", promo.Months,
		"offerExpires", offerExpires)

	return &TariffApplyResult{
		Success:      true,
		OfferExpires: &offerExpires,
	}
}


// Admin functions

// CreatePromoTariffCode создаёт новый промокод на тариф
func (s *TariffService) CreatePromoTariffCode(ctx context.Context, code string, price, devices, months, maxActivations, validHours int, adminID int64, validUntil *time.Time) (*database.PromoTariffCode, error) {
	// Validate input
	if errKey := ValidatePromoTariffCode(code, price, devices, months, maxActivations); errKey != "" {
		return nil, &ValidationError{Key: errKey}
	}

	if validHours <= 0 {
		return nil, &ValidationError{Key: "promo_tariff_invalid_valid_hours"}
	}

	code = strings.ToUpper(strings.TrimSpace(code))

	// Check if code already exists
	existing, err := s.promoTariffRepo.FindByCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing code: %w", err)
	}
	if existing != nil {
		return nil, &ValidationError{Key: "promo_tariff_code_exists"}
	}

	return s.promoTariffRepo.Create(ctx, code, price, devices, months, maxActivations, validHours, adminID, validUntil)
}

// GetAllPromoTariffCodes возвращает все промокоды на тариф с пагинацией
func (s *TariffService) GetAllPromoTariffCodes(ctx context.Context, limit, offset int) ([]database.PromoTariffCode, error) {
	return s.promoTariffRepo.GetAll(ctx, limit, offset)
}

// GetPromoTariffByID возвращает промокод по ID
func (s *TariffService) GetPromoTariffByID(ctx context.Context, id int64) (*database.PromoTariffCode, error) {
	return s.promoTariffRepo.FindByID(ctx, id)
}

// GetPromoTariffByCode возвращает промокод по коду
func (s *TariffService) GetPromoTariffByCode(ctx context.Context, code string) (*database.PromoTariffCode, error) {
	return s.promoTariffRepo.FindByCode(ctx, code)
}

// DeactivatePromoTariff деактивирует промокод
func (s *TariffService) DeactivatePromoTariff(ctx context.Context, promoID int64) error {
	return s.promoTariffRepo.SetActive(ctx, promoID, false)
}

// ActivatePromoTariff активирует промокод
func (s *TariffService) ActivatePromoTariff(ctx context.Context, promoID int64) error {
	return s.promoTariffRepo.SetActive(ctx, promoID, true)
}

// DeletePromoTariff удаляет промокод
func (s *TariffService) DeletePromoTariff(ctx context.Context, promoID int64) error {
	return s.promoTariffRepo.Delete(ctx, promoID)
}

// GetPromoTariffActivations возвращает все активации промокода
func (s *TariffService) GetPromoTariffActivations(ctx context.Context, promoID int64) ([]database.PromoTariffActivation, error) {
	return s.promoTariffRepo.GetActivationsByPromo(ctx, promoID)
}
