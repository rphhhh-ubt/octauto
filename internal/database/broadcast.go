package database

import (
	"context"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v4/pgxpool"
)

type BroadcastStatus string

const (
	BroadcastStatusPending    BroadcastStatus = "pending"
	BroadcastStatusInProgress BroadcastStatus = "in_progress"
	BroadcastStatusCompleted  BroadcastStatus = "completed"
	BroadcastStatusPartial    BroadcastStatus = "partial"
	BroadcastStatusFailed     BroadcastStatus = "failed"
)

type BroadcastHistory struct {
	ID          int64      `db:"id"`
	TargetType  string     `db:"target_type"`
	MessageText string     `db:"message_text"`
	TotalCount  int        `db:"total_count"`
	SentCount   int        `db:"sent_count"`
	FailedCount int        `db:"failed_count"`
	Status      string     `db:"status"`
	CreatedAt   time.Time  `db:"created_at"`
	CompletedAt *time.Time `db:"completed_at"`
}

type BroadcastRepository struct {
	pool *pgxpool.Pool
}

func NewBroadcastRepository(pool *pgxpool.Pool) *BroadcastRepository {
	return &BroadcastRepository{pool: pool}
}

func (br *BroadcastRepository) Create(ctx context.Context, targetType, messageText string) (int64, error) {
	query := sq.Insert("broadcast_history").
		Columns("target_type", "message_text", "status").
		Values(targetType, messageText, string(BroadcastStatusPending)).
		Suffix("RETURNING id").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return 0, err
	}

	var id int64
	err = br.pool.QueryRow(ctx, sql, args...).Scan(&id)
	return id, err
}

func (br *BroadcastRepository) UpdateStatus(ctx context.Context, id int64, status string, sentCount, failedCount int) error {
	now := time.Now()
	query := sq.Update("broadcast_history").
		Set("status", status).
		Set("sent_count", sentCount).
		Set("failed_count", failedCount).
		Set("completed_at", now).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return err
	}

	_, err = br.pool.Exec(ctx, sql, args...)
	return err
}

func (br *BroadcastRepository) UpdateProgress(ctx context.Context, id int64, sentCount, failedCount int) error {
	query := sq.Update("broadcast_history").
		Set("sent_count", sentCount).
		Set("failed_count", failedCount).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return err
	}

	_, err = br.pool.Exec(ctx, sql, args...)
	return err
}

func (br *BroadcastRepository) SetTotalCount(ctx context.Context, id int64, total int) error {
	query := sq.Update("broadcast_history").
		Set("total_count", total).
		Set("status", string(BroadcastStatusInProgress)).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return err
	}

	_, err = br.pool.Exec(ctx, sql, args...)
	return err
}

func (br *BroadcastRepository) List(ctx context.Context, limit, offset int) ([]BroadcastHistory, error) {
	query := sq.Select("id", "target_type", "message_text", "total_count", "sent_count", "failed_count", "status", "created_at", "completed_at").
		From("broadcast_history").
		OrderBy("created_at DESC").
		Limit(uint64(limit)).
		Offset(uint64(offset)).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := br.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []BroadcastHistory
	for rows.Next() {
		var h BroadcastHistory
		err := rows.Scan(&h.ID, &h.TargetType, &h.MessageText, &h.TotalCount, &h.SentCount, &h.FailedCount, &h.Status, &h.CreatedAt, &h.CompletedAt)
		if err != nil {
			return nil, err
		}
		history = append(history, h)
	}

	return history, rows.Err()
}

func (br *BroadcastRepository) FindByID(ctx context.Context, id int64) (*BroadcastHistory, error) {
	query := sq.Select("id", "target_type", "message_text", "total_count", "sent_count", "failed_count", "status", "created_at", "completed_at").
		From("broadcast_history").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, err
	}

	var h BroadcastHistory
	err = br.pool.QueryRow(ctx, sql, args...).Scan(&h.ID, &h.TargetType, &h.MessageText, &h.TotalCount, &h.SentCount, &h.FailedCount, &h.Status, &h.CreatedAt, &h.CompletedAt)
	if err != nil {
		return nil, err
	}

	return &h, nil
}

func (br *BroadcastRepository) Delete(ctx context.Context, id int64) error {
	query := sq.Delete("broadcast_history").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return err
	}

	_, err = br.pool.Exec(ctx, sql, args...)
	return err
}
