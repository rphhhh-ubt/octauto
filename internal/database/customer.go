package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"remnawave-tg-shop-bot/utils"
)

type CustomerRepository struct {
	pool *pgxpool.Pool
}

func NewCustomerRepository(poll *pgxpool.Pool) *CustomerRepository {
	return &CustomerRepository{pool: poll}
}

type Customer struct {
	ID               int64      `db:"id"`
	TelegramID       int64      `db:"telegram_id"`
	ExpireAt         *time.Time `db:"expire_at"`
	CreatedAt        time.Time  `db:"created_at"`
	SubscriptionLink *string    `db:"subscription_link"`
	Language         string     `db:"language"`

	// Trial inactive notification
	TrialInactiveNotifiedAt *time.Time `db:"trial_inactive_notified_at"`

	// Winback offer
	WinbackOfferSentAt    *time.Time `db:"winback_offer_sent_at"`
	WinbackOfferExpiresAt *time.Time `db:"winback_offer_expires_at"`
	WinbackOfferPrice     *int       `db:"winback_offer_price"`
	WinbackOfferDevices   *int       `db:"winback_offer_devices"`
	WinbackOfferMonths    *int       `db:"winback_offer_months"`

	// Recurring payments
	RecurringEnabled    bool       `db:"recurring_enabled"`
	PaymentMethodID     *string    `db:"payment_method_id"`
	RecurringTariffName *string    `db:"recurring_tariff_name"`
	RecurringMonths     *int       `db:"recurring_months"`
	RecurringAmount     *int       `db:"recurring_amount"`
	RecurringNotifiedAt *time.Time `db:"recurring_notified_at"`

	// Promo tariff offer
	PromoOfferPrice     *int       `db:"promo_offer_price"`
	PromoOfferDevices   *int       `db:"promo_offer_devices"`
	PromoOfferMonths    *int       `db:"promo_offer_months"`
	PromoOfferExpiresAt *time.Time `db:"promo_offer_expires_at"`
	PromoOfferCodeID    *int64     `db:"promo_offer_code_id"`
}

// customerColumns returns all customer columns for SELECT queries
func customerColumns() []string {
	return []string{
		"id", "telegram_id", "expire_at", "created_at", "subscription_link", "language",
		"trial_inactive_notified_at", "winback_offer_sent_at", "winback_offer_expires_at",
		"winback_offer_price", "winback_offer_devices", "winback_offer_months",
		"recurring_enabled", "payment_method_id", "recurring_tariff_name",
		"recurring_months", "recurring_amount", "recurring_notified_at",
		"promo_offer_price", "promo_offer_devices", "promo_offer_months",
		"promo_offer_expires_at", "promo_offer_code_id",
	}
}

// scanCustomer scans a row into a Customer struct
func scanCustomer(row pgx.Row) (*Customer, error) {
	var customer Customer
	err := row.Scan(
		&customer.ID,
		&customer.TelegramID,
		&customer.ExpireAt,
		&customer.CreatedAt,
		&customer.SubscriptionLink,
		&customer.Language,
		&customer.TrialInactiveNotifiedAt,
		&customer.WinbackOfferSentAt,
		&customer.WinbackOfferExpiresAt,
		&customer.WinbackOfferPrice,
		&customer.WinbackOfferDevices,
		&customer.WinbackOfferMonths,
		&customer.RecurringEnabled,
		&customer.PaymentMethodID,
		&customer.RecurringTariffName,
		&customer.RecurringMonths,
		&customer.RecurringAmount,
		&customer.RecurringNotifiedAt,
		&customer.PromoOfferPrice,
		&customer.PromoOfferDevices,
		&customer.PromoOfferMonths,
		&customer.PromoOfferExpiresAt,
		&customer.PromoOfferCodeID,
	)
	if err != nil {
		return nil, err
	}
	return &customer, nil
}

// scanCustomerFromRows scans rows into a Customer struct
func scanCustomerFromRows(rows pgx.Rows) (*Customer, error) {
	var customer Customer
	err := rows.Scan(
		&customer.ID,
		&customer.TelegramID,
		&customer.ExpireAt,
		&customer.CreatedAt,
		&customer.SubscriptionLink,
		&customer.Language,
		&customer.TrialInactiveNotifiedAt,
		&customer.WinbackOfferSentAt,
		&customer.WinbackOfferExpiresAt,
		&customer.WinbackOfferPrice,
		&customer.WinbackOfferDevices,
		&customer.WinbackOfferMonths,
		&customer.RecurringEnabled,
		&customer.PaymentMethodID,
		&customer.RecurringTariffName,
		&customer.RecurringMonths,
		&customer.RecurringAmount,
		&customer.RecurringNotifiedAt,
		&customer.PromoOfferPrice,
		&customer.PromoOfferDevices,
		&customer.PromoOfferMonths,
		&customer.PromoOfferExpiresAt,
		&customer.PromoOfferCodeID,
	)
	if err != nil {
		return nil, err
	}
	return &customer, nil
}

func (cr *CustomerRepository) FindByExpirationRange(ctx context.Context, startDate, endDate time.Time) (*[]Customer, error) {
	buildSelect := sq.Select(customerColumns()...).
		From("customer").
		Where(
			sq.And{
				sq.NotEq{"expire_at": nil},
				sq.GtOrEq{"expire_at": startDate},
				sq.LtOrEq{"expire_at": endDate},
			},
		).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query customers by expiration range: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		customer, err := scanCustomerFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan customer row: %w", err)
		}
		customers = append(customers, *customer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over customer rows: %w", err)
	}

	return &customers, nil
}

func (cr *CustomerRepository) FindById(ctx context.Context, id int64) (*Customer, error) {
	buildSelect := sq.Select(customerColumns()...).
		From("customer").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	customer, err := scanCustomer(cr.pool.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query customer: %w", err)
	}
	return customer, nil
}

func (cr *CustomerRepository) FindByTelegramId(ctx context.Context, telegramId int64) (*Customer, error) {
	buildSelect := sq.Select(customerColumns()...).
		From("customer").
		Where(sq.Eq{"telegram_id": telegramId}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	customer, err := scanCustomer(cr.pool.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query customer: %w", err)
	}
	return customer, nil
}

func (cr *CustomerRepository) Create(ctx context.Context, customer *Customer) (*Customer, error) {
	return cr.FindOrCreate(ctx, customer)
}

// FindOrCreate создаёт нового customer или возвращает существующего (защита от duplicate key при параллельных запросах)
func (cr *CustomerRepository) FindOrCreate(ctx context.Context, customer *Customer) (*Customer, error) {
	query := `
		INSERT INTO customer (telegram_id, expire_at, language)
		VALUES ($1, $2, $3)
		ON CONFLICT (telegram_id) DO UPDATE SET telegram_id = customer.telegram_id
		RETURNING ` + strings.Join(customerColumns(), ", ")

	row := cr.pool.QueryRow(ctx, query, customer.TelegramID, customer.ExpireAt, customer.Language)
	result, err := scanCustomer(row)
	if err != nil {
		return nil, fmt.Errorf("failed to find or create customer: %w", err)
	}

	slog.Info("user found or created in bot database", "telegramId", utils.MaskHalfInt64(result.TelegramID))
	return result, nil
}

func (cr *CustomerRepository) UpdateFields(ctx context.Context, id int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	buildUpdate := sq.Update("customer").
		PlaceholderFormat(sq.Dollar).
		Where(sq.Eq{"id": id})

	for field, value := range updates {
		buildUpdate = buildUpdate.Set(field, value)
	}

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	result, err := cr.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to update customer: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("no customer found with id: %s", utils.MaskHalfInt64(id))
	}

	return nil
}

func (cr *CustomerRepository) FindByTelegramIds(ctx context.Context, telegramIDs []int64) ([]Customer, error) {
	buildSelect := sq.Select(customerColumns()...).
		From("customer").
		Where(sq.Eq{"telegram_id": telegramIDs}).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query customers: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		customer, err := scanCustomerFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan customer row: %w", err)
		}
		customers = append(customers, *customer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over customer rows: %w", err)
	}

	return customers, nil
}

func (cr *CustomerRepository) CreateBatch(ctx context.Context, customers []Customer) error {
	if len(customers) == 0 {
		return nil
	}
	builder := sq.Insert("customer").
		Columns("telegram_id", "expire_at", "language", "subscription_link").
		PlaceholderFormat(sq.Dollar)
	for _, cust := range customers {
		builder = builder.Values(cust.TelegramID, cust.ExpireAt, cust.Language, cust.SubscriptionLink)
	}
	sqlStr, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build batch insert query: %w", err)
	}

	_, err = cr.pool.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("failed to execute batch insert: %w", err)
	}

	return nil
}

func (cr *CustomerRepository) UpdateBatch(ctx context.Context, customers []Customer) error {
	if len(customers) == 0 {
		return nil
	}
	query := "UPDATE customer SET expire_at = c.expire_at, subscription_link = c.subscription_link FROM (VALUES "
	var args []interface{}
	for i, cust := range customers {
		if i > 0 {
			query += ", "
		}
		query += fmt.Sprintf("($%d::bigint, $%d::timestamp, $%d::text)", i*3+1, i*3+2, i*3+3)
		args = append(args, cust.TelegramID, cust.ExpireAt, cust.SubscriptionLink)
	}
	query += ") AS c(telegram_id, expire_at, subscription_link) WHERE customer.telegram_id = c.telegram_id"

	_, err := cr.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to execute batch update: %w", err)
	}

	return nil
}

func (cr *CustomerRepository) DeleteByNotInTelegramIds(ctx context.Context, telegramIDs []int64) error {
	var buildDelete sq.DeleteBuilder
	if len(telegramIDs) == 0 {
		buildDelete = sq.Delete("customer")
	} else {
		buildDelete = sq.Delete("customer").
			PlaceholderFormat(sq.Dollar).
			Where(sq.NotEq{"telegram_id": telegramIDs})
	}

	sqlStr, args, err := buildDelete.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build delete query: %w", err)
	}

	_, err = cr.pool.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("failed to delete customers: %w", err)
	}

	return nil

}

func (cr *CustomerRepository) FindAll(ctx context.Context) ([]Customer, error) {
	buildSelect := sq.Select(customerColumns()...).
		From("customer").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query all customers: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		customer, err := scanCustomerFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan customer row: %w", err)
		}
		customers = append(customers, *customer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error after scanning all rows: %w", err)
	}

	return customers, nil
}

func (cr *CustomerRepository) UpdateExpireAt(ctx context.Context, id int64, expireAt time.Time) error {
	buildUpdate := sq.Update("customer").
		Set("expire_at", expireAt).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update expire_at query: %w", err)
	}

	_, err = cr.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to update expire_at: %w", err)
	}
	return nil
}


// FindTrialUsersForInactiveNotification находит ТОЛЬКО триальных пользователей (без оплаченных покупок)
// Условия: триал начался от 1 до 2 часов назад, уведомление ещё не отправлялось, НЕТ оплаченных покупок
func (cr *CustomerRepository) FindTrialUsersForInactiveNotification(ctx context.Context) ([]Customer, error) {
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)
	twoHoursAgo := now.Add(-2 * time.Hour)

	// Используем raw SQL для LEFT JOIN — только пользователи БЕЗ оплаченных покупок (триальные)
	// Окно: созданы от 1 до 2 часов назад (чтобы не спамить старым пользователям)
	query := `
		SELECT c.id, c.telegram_id, c.expire_at, c.created_at, c.subscription_link, c.language,
			   c.trial_inactive_notified_at, c.winback_offer_sent_at, c.winback_offer_expires_at,
			   c.winback_offer_price, c.winback_offer_devices, c.winback_offer_months,
			   c.recurring_enabled, c.payment_method_id, c.recurring_tariff_name,
			   c.recurring_months, c.recurring_amount, c.recurring_notified_at,
			   c.promo_offer_price, c.promo_offer_devices, c.promo_offer_months,
			   c.promo_offer_expires_at, c.promo_offer_code_id
		FROM customer c
		LEFT JOIN purchase p ON p.customer_id = c.id AND p.status = 'paid'
		WHERE c.expire_at IS NOT NULL
		  AND c.expire_at > $1
		  AND c.created_at <= $2
		  AND c.created_at >= $3
		  AND c.trial_inactive_notified_at IS NULL
		GROUP BY c.id
		HAVING COUNT(p.id) = 0
	`

	rows, err := cr.pool.Query(ctx, query, now, oneHourAgo, twoHoursAgo)
	if err != nil {
		return nil, fmt.Errorf("failed to query trial users for inactive notification: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		customer, err := scanCustomerFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan customer row: %w", err)
		}
		customers = append(customers, *customer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over customer rows: %w", err)
	}

	return customers, nil
}

// FindExpiredTrialUsersForWinback находит ТОЛЬКО триальных пользователей (без оплаченных покупок) для winback
// Условия: триал истёк от 24 до 48 часов назад, предложение ещё не отправлялось, НЕТ оплаченных покупок
func (cr *CustomerRepository) FindExpiredTrialUsersForWinback(ctx context.Context) ([]Customer, error) {
	now := time.Now()
	oneDayAgo := now.Add(-24 * time.Hour)
	twoDaysAgo := now.Add(-48 * time.Hour)

	// Используем raw SQL для LEFT JOIN — только пользователи БЕЗ оплаченных покупок (триальные)
	query := `
		SELECT c.id, c.telegram_id, c.expire_at, c.created_at, c.subscription_link, c.language,
			   c.trial_inactive_notified_at, c.winback_offer_sent_at, c.winback_offer_expires_at,
			   c.winback_offer_price, c.winback_offer_devices, c.winback_offer_months,
			   c.recurring_enabled, c.payment_method_id, c.recurring_tariff_name,
			   c.recurring_months, c.recurring_amount, c.recurring_notified_at,
			   c.promo_offer_price, c.promo_offer_devices, c.promo_offer_months,
			   c.promo_offer_expires_at, c.promo_offer_code_id
		FROM customer c
		LEFT JOIN purchase p ON p.customer_id = c.id AND p.status = 'paid'
		WHERE c.expire_at IS NOT NULL
		  AND c.expire_at <= $1
		  AND c.expire_at >= $2
		  AND c.winback_offer_sent_at IS NULL
		GROUP BY c.id
		HAVING COUNT(p.id) = 0
	`

	rows, err := cr.pool.Query(ctx, query, oneDayAgo, twoDaysAgo)
	if err != nil {
		return nil, fmt.Errorf("failed to query expired trial users for winback: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		customer, err := scanCustomerFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan customer row: %w", err)
		}
		customers = append(customers, *customer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over customer rows: %w", err)
	}

	return customers, nil
}

// UpdateTrialInactiveNotifiedAt обновляет время отправки уведомления о неактивности
func (cr *CustomerRepository) UpdateTrialInactiveNotifiedAt(ctx context.Context, id int64, notifiedAt time.Time) error {
	buildUpdate := sq.Update("customer").
		Set("trial_inactive_notified_at", notifiedAt).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	_, err = cr.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to update trial_inactive_notified_at: %w", err)
	}
	return nil
}

// UpdateWinbackOffer обновляет информацию о winback предложении
func (cr *CustomerRepository) UpdateWinbackOffer(ctx context.Context, id int64, sentAt, expiresAt time.Time, price, devices, months int) error {
	buildUpdate := sq.Update("customer").
		Set("winback_offer_sent_at", sentAt).
		Set("winback_offer_expires_at", expiresAt).
		Set("winback_offer_price", price).
		Set("winback_offer_devices", devices).
		Set("winback_offer_months", months).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	_, err = cr.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to update winback offer: %w", err)
	}
	return nil
}

// FindCustomersWithRecurringEnabled находит всех пользователей с включённым автопродлением
func (cr *CustomerRepository) FindCustomersWithRecurringEnabled(ctx context.Context) ([]Customer, error) {
	buildSelect := sq.Select(customerColumns()...).
		From("customer").
		Where(sq.Eq{"recurring_enabled": true}).
		Where(sq.NotEq{"payment_method_id": nil}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query customers with recurring enabled: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		customer, err := scanCustomerFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan customer row: %w", err)
		}
		customers = append(customers, *customer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over customer rows: %w", err)
	}

	return customers, nil
}

// UpdateRecurringSettings обновляет настройки автопродления для пользователя
func (cr *CustomerRepository) UpdateRecurringSettings(ctx context.Context, id int64, enabled bool, paymentMethodID *string, tariffName *string, months *int, amount *int) error {
	buildUpdate := sq.Update("customer").
		Set("recurring_enabled", enabled).
		Set("payment_method_id", paymentMethodID).
		Set("recurring_tariff_name", tariffName).
		Set("recurring_months", months).
		Set("recurring_amount", amount).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	_, err = cr.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to update recurring settings: %w", err)
	}
	return nil
}

// DisableRecurring отключает автопродление, но сохраняет payment_method_id
// Это позволяет пользователю легко включить автопродление обратно
func (cr *CustomerRepository) DisableRecurring(ctx context.Context, id int64) error {
	buildUpdate := sq.Update("customer").
		Set("recurring_enabled", false).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	_, err = cr.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to disable recurring: %w", err)
	}
	return nil
}

// DeletePaymentMethod удаляет сохранённый способ оплаты и отключает автопродление
func (cr *CustomerRepository) DeletePaymentMethod(ctx context.Context, id int64) error {
	buildUpdate := sq.Update("customer").
		Set("recurring_enabled", false).
		Set("payment_method_id", nil).
		Set("recurring_amount", nil).
		Set("recurring_months", nil).
		Set("recurring_tariff_name", nil).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	_, err = cr.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to delete payment method: %w", err)
	}
	return nil
}

// UpdateRecurringNotifiedAt обновляет время последнего уведомления о предстоящем списании
func (cr *CustomerRepository) UpdateRecurringNotifiedAt(ctx context.Context, id int64, notifiedAt time.Time) error {
	buildUpdate := sq.Update("customer").
		Set("recurring_notified_at", notifiedAt).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	_, err = cr.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to update recurring_notified_at: %w", err)
	}
	return nil
}


// UpdatePromoOffer обновляет информацию о promo tariff предложении
func (cr *CustomerRepository) UpdatePromoOffer(ctx context.Context, id int64, price, devices, months int, expiresAt time.Time, codeID int64) error {
	buildUpdate := sq.Update("customer").
		Set("promo_offer_price", price).
		Set("promo_offer_devices", devices).
		Set("promo_offer_months", months).
		Set("promo_offer_expires_at", expiresAt).
		Set("promo_offer_code_id", codeID).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	_, err = cr.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to update promo offer: %w", err)
	}
	return nil
}

// ClearPromoOffer очищает promo tariff предложение после покупки
func (cr *CustomerRepository) ClearPromoOffer(ctx context.Context, id int64) error {
	buildUpdate := sq.Update("customer").
		Set("promo_offer_price", nil).
		Set("promo_offer_devices", nil).
		Set("promo_offer_months", nil).
		Set("promo_offer_expires_at", nil).
		Set("promo_offer_code_id", nil).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build clear promo offer query: %w", err)
	}

	_, err = cr.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to clear promo offer: %w", err)
	}
	return nil
}

// HasActivePromoOffer проверяет, есть ли у пользователя активное promo tariff предложение
func HasActivePromoOffer(customer *Customer) bool {
	if customer == nil {
		return false
	}
	if customer.PromoOfferPrice == nil || customer.PromoOfferExpiresAt == nil {
		return false
	}
	return customer.PromoOfferExpiresAt.After(time.Now())
}

// HasActiveWinbackOffer проверяет, есть ли у пользователя активное winback предложение
func HasActiveWinbackOffer(customer *Customer) bool {
	if customer == nil {
		return false
	}
	if customer.WinbackOfferSentAt == nil {
		return false
	}
	// Проверяем что предложение не истекло (если есть срок)
	if customer.WinbackOfferExpiresAt != nil {
		return customer.WinbackOfferExpiresAt.After(time.Now())
	}
	return true
}

// ClearWinbackOffer очищает winback предложение после покупки
func (cr *CustomerRepository) ClearWinbackOffer(ctx context.Context, id int64) error {
	buildUpdate := sq.Update("customer").
		Set("winback_offer_sent_at", nil).
		Set("winback_offer_expires_at", nil).
		Set("winback_offer_price", nil).
		Set("winback_offer_devices", nil).
		Set("winback_offer_months", nil).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build clear winback offer query: %w", err)
	}

	_, err = cr.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to clear winback offer: %w", err)
	}
	return nil
}
