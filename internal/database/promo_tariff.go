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
	ErrPromoTariffNotFound      = errors.New("promo tariff code not found")
	ErrPromoTariffAlreadyUsed   = errors.New("promo tariff code already used by this user")
	ErrPromoTariffExpired       = errors.New("promo tariff code expired")
	ErrPromoTariffLimitReached  = errors.New("promo tariff code activation limit reached")
	ErrPromoTariffInactive      = errors.New("promo tariff code is inactive")
	ErrPromoTariffInvalidFormat = errors.New("invalid promo tariff code format")
)

type PromoTariffCode struct {
	ID                 int64      `db:"id"`
	Code               string     `db:"code"`
	Price              int        `db:"price"`
	Devices            int        `db:"devices"`
	Months             int        `db:"months"`
	MaxActivations     int        `db:"max_activations"`
	CurrentActivations int        `db:"current_activations"`
	ValidHours         int        `db:"valid_hours"`
	IsActive           bool       `db:"is_active"`
	CreatedByAdminID   int64      `db:"created_by_admin_id"`
	CreatedAt          time.Time  `db:"created_at"`
	ValidUntil         *time.Time `db:"valid_until"`
}

type PromoTariffActivation struct {
	ID            int64     `db:"id"`
	PromoTariffID int64     `db:"promo_tariff_id"`
	CustomerID    int64     `db:"customer_id"`
	ActivatedAt   time.Time `db:"activated_at"`
}

type PromoTariffRepository struct {
	pool *pgxpool.Pool
}

func NewPromoTariffRepository(pool *pgxpool.Pool) *PromoTariffRepository {
	return &PromoTariffRepository{pool: pool}
}


// Create создаёт новый промокод на тариф
func (r *PromoTariffRepository) Create(ctx context.Context, code string, price, devices, months, maxActivations, validHours int, adminID int64, validUntil *time.Time) (*PromoTariffCode, error) {
	code = strings.ToUpper(strings.TrimSpace(code))

	columns := []string{"code", "price", "devices", "months", "max_activations", "valid_hours", "created_by_admin_id"}
	values := []interface{}{code, price, devices, months, maxActivations, validHours, adminID}

	if validUntil != nil {
		columns = append(columns, "valid_until")
		values = append(values, validUntil)
	}

	builder := sq.Insert("promo_tariff_code").
		Columns(columns...).
		Values(values...).
		Suffix("RETURNING id, code, price, devices, months, max_activations, current_activations, valid_hours, is_active, created_by_admin_id, created_at, valid_until").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build insert promo tariff query: %w", err)
	}

	row := r.pool.QueryRow(ctx, sql, args...)
	var promo PromoTariffCode
	if err := row.Scan(&promo.ID, &promo.Code, &promo.Price, &promo.Devices, &promo.Months,
		&promo.MaxActivations, &promo.CurrentActivations, &promo.ValidHours, &promo.IsActive,
		&promo.CreatedByAdminID, &promo.CreatedAt, &promo.ValidUntil); err != nil {
		return nil, fmt.Errorf("failed to create promo tariff code: %w", err)
	}
	return &promo, nil
}

// FindByCode находит промокод по коду
func (r *PromoTariffRepository) FindByCode(ctx context.Context, code string) (*PromoTariffCode, error) {
	code = strings.ToUpper(strings.TrimSpace(code))

	query := sq.Select("id", "code", "price", "devices", "months", "max_activations", "current_activations",
		"valid_hours", "is_active", "created_by_admin_id", "created_at", "valid_until").
		From("promo_tariff_code").
		Where(sq.Eq{"code": code}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select promo tariff query: %w", err)
	}

	var promo PromoTariffCode
	err = r.pool.QueryRow(ctx, sql, args...).Scan(&promo.ID, &promo.Code, &promo.Price, &promo.Devices,
		&promo.Months, &promo.MaxActivations, &promo.CurrentActivations, &promo.ValidHours,
		&promo.IsActive, &promo.CreatedByAdminID, &promo.CreatedAt, &promo.ValidUntil)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find promo tariff code: %w", err)
	}
	return &promo, nil
}

// FindByID находит промокод по ID
func (r *PromoTariffRepository) FindByID(ctx context.Context, id int64) (*PromoTariffCode, error) {
	query := sq.Select("id", "code", "price", "devices", "months", "max_activations", "current_activations",
		"valid_hours", "is_active", "created_by_admin_id", "created_at", "valid_until").
		From("promo_tariff_code").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select promo tariff by id query: %w", err)
	}

	var promo PromoTariffCode
	err = r.pool.QueryRow(ctx, sql, args...).Scan(&promo.ID, &promo.Code, &promo.Price, &promo.Devices,
		&promo.Months, &promo.MaxActivations, &promo.CurrentActivations, &promo.ValidHours,
		&promo.IsActive, &promo.CreatedByAdminID, &promo.CreatedAt, &promo.ValidUntil)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find promo tariff code by id: %w", err)
	}
	return &promo, nil
}


// GetAll возвращает все промокоды на тариф с пагинацией
func (r *PromoTariffRepository) GetAll(ctx context.Context, limit, offset int) ([]PromoTariffCode, error) {
	query := sq.Select("id", "code", "price", "devices", "months", "max_activations", "current_activations",
		"valid_hours", "is_active", "created_by_admin_id", "created_at", "valid_until").
		From("promo_tariff_code").
		OrderBy("created_at DESC").
		Limit(uint64(limit)).
		Offset(uint64(offset)).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select all promo tariffs query: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query promo tariffs: %w", err)
	}
	defer rows.Close()

	var list []PromoTariffCode
	for rows.Next() {
		var promo PromoTariffCode
		if err := rows.Scan(&promo.ID, &promo.Code, &promo.Price, &promo.Devices, &promo.Months,
			&promo.MaxActivations, &promo.CurrentActivations, &promo.ValidHours, &promo.IsActive,
			&promo.CreatedByAdminID, &promo.CreatedAt, &promo.ValidUntil); err != nil {
			return nil, fmt.Errorf("failed to scan promo tariff row: %w", err)
		}
		list = append(list, promo)
	}
	return list, nil
}

// SetActive активирует или деактивирует промокод
func (r *PromoTariffRepository) SetActive(ctx context.Context, id int64, isActive bool) error {
	query := sq.Update("promo_tariff_code").
		Set("is_active", isActive).
		Where(sq.Eq{"id": id}).
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

// Delete удаляет промокод
func (r *PromoTariffRepository) Delete(ctx context.Context, id int64) error {
	query := sq.Delete("promo_tariff_code").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build delete query: %w", err)
	}

	_, err = r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to delete promo tariff: %w", err)
	}
	return nil
}

// IncrementActivations увеличивает счётчик активаций
func (r *PromoTariffRepository) IncrementActivations(ctx context.Context, id int64) error {
	query := sq.Update("promo_tariff_code").
		Set("current_activations", sq.Expr("current_activations + 1")).
		Where(sq.Eq{"id": id}).
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


// IsUsedByCustomer проверяет, использовал ли пользователь этот промокод
func (r *PromoTariffRepository) IsUsedByCustomer(ctx context.Context, promoTariffID, customerID int64) (bool, error) {
	query := sq.Select("1").
		From("promo_tariff_activation").
		Where(sq.Eq{"promo_tariff_id": promoTariffID, "customer_id": customerID}).
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

// RecordActivation записывает активацию промокода пользователем
func (r *PromoTariffRepository) RecordActivation(ctx context.Context, promoTariffID, customerID int64) error {
	query := sq.Insert("promo_tariff_activation").
		Columns("promo_tariff_id", "customer_id").
		Values(promoTariffID, customerID).
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

// GetActivationsByPromo возвращает все активации для промокода
func (r *PromoTariffRepository) GetActivationsByPromo(ctx context.Context, promoTariffID int64) ([]PromoTariffActivation, error) {
	query := sq.Select("id", "promo_tariff_id", "customer_id", "activated_at").
		From("promo_tariff_activation").
		Where(sq.Eq{"promo_tariff_id": promoTariffID}).
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

	var list []PromoTariffActivation
	for rows.Next() {
		var act PromoTariffActivation
		if err := rows.Scan(&act.ID, &act.PromoTariffID, &act.CustomerID, &act.ActivatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan activation row: %w", err)
		}
		list = append(list, act)
	}
	return list, nil
}
