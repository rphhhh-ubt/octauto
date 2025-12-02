package promo

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
)

var promoCodeRegex = regexp.MustCompile(`^[A-Z0-9_-]{3,50}$`)

type Service struct {
	promoRepo      *database.PromoRepository
	customerRepo   *database.CustomerRepository
	remnawaveClient *remnawave.Client
}

func NewService(
	promoRepo *database.PromoRepository,
	customerRepo *database.CustomerRepository,
	remnawaveClient *remnawave.Client,
) *Service {
	return &Service{
		promoRepo:       promoRepo,
		customerRepo:    customerRepo,
		remnawaveClient: remnawaveClient,
	}
}

type ApplyResult struct {
	Success    bool
	NewExpire  *time.Time
	BonusDays  int
	ErrorKey   string // translation key for error message
}

func (s *Service) ApplyPromoCode(ctx context.Context, customerID int64, telegramID int64, code string) *ApplyResult {
	code = strings.ToUpper(strings.TrimSpace(code))
	
	// Validate format
	if !promoCodeRegex.MatchString(code) {
		return &ApplyResult{Success: false, ErrorKey: "promo_invalid_format"}
	}

	// Find promo code
	promo, err := s.promoRepo.FindByCode(ctx, code)
	if err != nil {
		slog.Error("Error finding promo code", "code", code, "error", err)
		return &ApplyResult{Success: false, ErrorKey: "promo_error"}
	}
	if promo == nil {
		return &ApplyResult{Success: false, ErrorKey: "promo_not_found"}
	}

	// Check if active
	if !promo.IsActive {
		return &ApplyResult{Success: false, ErrorKey: "promo_inactive"}
	}

	// Check expiration
	if promo.ValidUntil != nil && time.Now().After(*promo.ValidUntil) {
		return &ApplyResult{Success: false, ErrorKey: "promo_expired"}
	}

	// Check activation limit
	if promo.CurrentActivations >= promo.MaxActivations {
		return &ApplyResult{Success: false, ErrorKey: "promo_limit_reached"}
	}

	// Check if already used by this customer
	used, err := s.promoRepo.IsUsedByCustomer(ctx, promo.ID, customerID)
	if err != nil {
		slog.Error("Error checking promo usage", "promoID", promo.ID, "customerID", customerID, "error", err)
		return &ApplyResult{Success: false, ErrorKey: "promo_error"}
	}
	if used {
		return &ApplyResult{Success: false, ErrorKey: "promo_already_used"}
	}

	// Apply bonus days via Remnawave API
	ctxWithUsername := ctx
	if username := ctx.Value("username"); username == nil {
		ctxWithUsername = context.WithValue(ctx, "username", "")
	}
	
	newExpire, err := s.remnawaveClient.CreateOrUpdateUser(
		ctxWithUsername,
		customerID,
		telegramID,
		config.TrafficLimit(),
		promo.BonusDays,
		false,
	)
	if err != nil {
		slog.Error("Error applying promo bonus", "telegramID", telegramID, "bonusDays", promo.BonusDays, "error", err)
		return &ApplyResult{Success: false, ErrorKey: "promo_apply_error"}
	}

	// Record activation
	if err := s.promoRepo.RecordActivation(ctx, promo.ID, customerID); err != nil {
		slog.Error("Error recording promo activation", "promoID", promo.ID, "customerID", customerID, "error", err)
		// Don't fail - bonus already applied
	}

	// Increment counter
	if err := s.promoRepo.IncrementActivations(ctx, promo.ID); err != nil {
		slog.Error("Error incrementing promo activations", "promoID", promo.ID, "error", err)
	}

	// Update customer expire_at
	if newExpire == nil {
		slog.Error("Remnawave returned nil user after promo apply", "customerID", customerID)
		return &ApplyResult{Success: false, ErrorKey: "promo_apply_error"}
	}

	if err := s.customerRepo.UpdateExpireAt(ctx, customerID, newExpire.ExpireAt); err != nil {
		slog.Error("Error updating customer expire_at", "customerID", customerID, "error", err)
	}

	slog.Info("Promo code applied", "code", code, "customerID", customerID, "bonusDays", promo.BonusDays)

	expireAt := newExpire.ExpireAt
	return &ApplyResult{
		Success:   true,
		NewExpire: &expireAt,
		BonusDays: promo.BonusDays,
	}
}

// Admin functions

func (s *Service) CreatePromoCode(ctx context.Context, code string, bonusDays, maxActivations int, adminID int64, validUntil *time.Time) (*database.PromoCode, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	
	if !promoCodeRegex.MatchString(code) {
		return nil, database.ErrPromoInvalidFormat
	}

	existing, err := s.promoRepo.FindByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("promo code already exists")
	}

	return s.promoRepo.Create(ctx, code, bonusDays, maxActivations, adminID, validUntil)
}

func (s *Service) GetAllPromoCodes(ctx context.Context, limit, offset int) ([]database.PromoCode, error) {
	return s.promoRepo.GetAll(ctx, limit, offset)
}

func (s *Service) GetPromoByID(ctx context.Context, id int64) (*database.PromoCode, error) {
	return s.promoRepo.FindByID(ctx, id)
}

func (s *Service) DeactivatePromo(ctx context.Context, promoID int64) error {
	return s.promoRepo.SetActive(ctx, promoID, false)
}

func (s *Service) ActivatePromo(ctx context.Context, promoID int64) error {
	return s.promoRepo.SetActive(ctx, promoID, true)
}

func (s *Service) DeletePromo(ctx context.Context, promoID int64) error {
	return s.promoRepo.Delete(ctx, promoID)
}

func (s *Service) GetPromoActivations(ctx context.Context, promoID int64) ([]database.PromoCodeActivation, error) {
	return s.promoRepo.GetActivationsByPromo(ctx, promoID)
}
