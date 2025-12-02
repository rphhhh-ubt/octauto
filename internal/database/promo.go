package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

var (
	ErrPromoNotFound       = errors.New("promo code not found")
	ErrPromoAlreadyUsed    = errors.New("promo code already used by this user")
	ErrPromoExpired        = errors.New("promo code expired")
	ErrPromoLimitReached   = errors.New("promo code activation limit reached")
	ErrPromoInactive       = errors.New("promo code is inactive")
	ErrPromoInvalidFormat  = errors.New("invalid promo code format")
)

type PromoCode struct {
	ID                 int64      `db:"id"`
	Code               string     `db:"code"`
	BonusDays          int        `db:"bonus_days"`
	MaxActivations     int        `db:"max_activations"`
	CurrentActivations int        `db:"current_activations"`
	IsActive           bool       `db:"is_active"`
	CreatedByAdminID   int64      `db:"created_by_admin_id"`
	CreatedAt          time.Time  `db:"created_at"`
	ValidUntil         *time.Time `db:"valid_until"`
}

type PromoCodeActivation struct {
	ID          int64     `db:"id"`
	PromoCodeID int64     `db:"promo_code_id"`
	CustomerID  int64     `db:"customer_id"`
	ActivatedAt time.Time `db:"activated_at"`
}

type PromoRepository struct {
	pool *pgxpool.Pool
}

func NewPromoRepository(pool *pgxpool.Pool) *PromoRepository {
	return &PromoRepository{pool: pool}
}

func (r *PromoRepository) Create(ctx context.Context, code string, bonusDays, maxActivations int, adminID int64, validUntil *time.Time) (*PromoCode, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	
	builder := sq.Insert("promo_code").
		Columns("code", "bonus_days", "max_activations", "created_by_admin_id").
		Values(code, bonusDays, maxActivations, adminID).
		Suffix("RETURNING id, code, bonus_days, max_activations, current_activations, is_active, created_by_admin_id, created_at, valid_until").
		PlaceholderFormat(sq.Dollar)

	if validUntil != nil {
		builder = sq.Insert("promo_code").
			Columns("code", "bonus_days", "max_activations", "created_by_admin_id", "valid_until").
			Values(code, bonusDays, maxActivations, adminID, validUntil).
			Suffix("RETURNING id, code, bonus_days, max_activations, current_activations, is_active, created_by_admin_id, created_at, valid_until").
			PlaceholderFormat(sq.Dollar)
	}

	sql, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build insert promo query: %w", err)
	}

	row := r.pool.QueryRow(ctx, sql, args...)
	var promo PromoCode
	if err := row.Scan(&promo.ID, &promo.Code, &promo.BonusDays, &promo.MaxActivations, 
		&promo.CurrentActivations, &promo.IsActive, &promo.CreatedByAdminID, &promo.CreatedAt, &promo.ValidUntil); err != nil {
		return nil, fmt.Errorf("failed to create promo code: %w", err)
	}
	return &promo, nil
}

func (r *PromoRepository) FindByCode(ctx context.Context, code string) (*PromoCode, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	
	query := sq.Select("id", "code", "bonus_days", "max_activations", "current_activations", 
		"is_active", "created_by_admin_id", "created_at", "valid_until").
		From("promo_code").
		Where(sq.Eq{"code": code}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select promo query: %w", err)
	}

	var promo PromoCode
	err = r.pool.QueryRow(ctx, sql, args...).Scan(&promo.ID, &promo.Code, &promo.BonusDays, 
		&promo.MaxActivations, &promo.CurrentActivations, &promo.IsActive, 
		&promo.CreatedByAdminID, &promo.CreatedAt, &promo.ValidUntil)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find promo code: %w", err)
	}
	return &promo, nil
}

func (r *PromoRepository) FindByID(ctx context.Context, id int64) (*PromoCode, error) {
	query := sq.Select("id", "code", "bonus_days", "max_activations", "current_activations", 
		"is_active", "created_by_admin_id", "created_at", "valid_until").
		From("promo_code").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select promo by id query: %w", err)
	}

	var promo PromoCode
	err = r.pool.QueryRow(ctx, sql, args...).Scan(&promo.ID, &promo.Code, &promo.BonusDays, 
		&promo.MaxActivations, &promo.CurrentActivations, &promo.IsActive, 
		&promo.CreatedByAdminID, &promo.CreatedAt, &promo.ValidUntil)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find promo code by id: %w", err)
	}
	return &promo, nil
}

func (r *PromoRepository) GetAll(ctx context.Context, limit, offset int) ([]PromoCode, error) {
	query := sq.Select("id", "code", "bonus_days", "max_activations", "current_activations", 
		"is_active", "created_by_admin_id", "created_at", "valid_until").
		From("promo_code").
		OrderBy("created_at DESC").
		Limit(uint64(limit)).
		Offset(uint64(offset)).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select all promos query: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query promos: %w", err)
	}
	defer rows.Close()

	var list []PromoCode
	for rows.Next() {
		var promo PromoCode
		if err := rows.Scan(&promo.ID, &promo.Code, &promo.BonusDays, &promo.MaxActivations, 
			&promo.CurrentActivations, &promo.IsActive, &promo.CreatedByAdminID, 
			&promo.CreatedAt, &promo.ValidUntil); err != nil {
			return nil, fmt.Errorf("failed to scan promo row: %w", err)
		}
		list = append(list, promo)
	}
	return list, nil
}

func (r *PromoRepository) IsUsedByCustomer(ctx context.Context, promoID, customerID int64) (bool, error) {
	query := sq.Select("1").
		From("promo_code_activation").
		Where(sq.Eq{"promo_code_id": promoID, "customer_id": customerID}).
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return false, fmt.Errorf("failed to build check activation query: %w", err)
	}

	var exists int
	err = r.pool.QueryRow(ctx, sql, args...).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check activation: %w", err)
	}
	return true, nil
}

func (r *PromoRepository) RecordActivation(ctx context.Context, promoID, customerID int64) error {
	query := sq.Insert("promo_code_activation").
		Columns("promo_code_id", "customer_id").
		Values(promoID, customerID).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build insert activation query: %w", err)
	}

	_, err = r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to record activation: %w", err)
	}
	return nil
}

func (r *PromoRepository) IncrementActivations(ctx context.Context, promoID int64) error {
	query := sq.Update("promo_code").
		Set("current_activations", sq.Expr("current_activations + 1")).
		Where(sq.Eq{"id": promoID}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build increment query: %w", err)
	}

	_, err = r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to increment activations: %w", err)
	}
	return nil
}

func (r *PromoRepository) SetActive(ctx context.Context, promoID int64, isActive bool) error {
	query := sq.Update("promo_code").
		Set("is_active", isActive).
		Where(sq.Eq{"id": promoID}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build set active query: %w", err)
	}

	_, err = r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to set active: %w", err)
	}
	return nil
}

func (r *PromoRepository) Delete(ctx context.Context, promoID int64) error {
	query := sq.Delete("promo_code").
		Where(sq.Eq{"id": promoID}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build delete query: %w", err)
	}

	_, err = r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to delete promo: %w", err)
	}
	return nil
}

func (r *PromoRepository) GetActivationsByPromo(ctx context.Context, promoID int64) ([]PromoCodeActivation, error) {
	query := sq.Select("id", "promo_code_id", "customer_id", "activated_at").
		From("promo_code_activation").
		Where(sq.Eq{"promo_code_id": promoID}).
		OrderBy("activated_at DESC").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select activations query: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query activations: %w", err)
	}
	defer rows.Close()

	var list []PromoCodeActivation
	for rows.Next() {
		var act PromoCodeActivation
		if err := rows.Scan(&act.ID, &act.PromoCodeID, &act.CustomerID, &act.ActivatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan activation row: %w", err)
		}
		list = append(list, act)
	}
	return list, nil
}
