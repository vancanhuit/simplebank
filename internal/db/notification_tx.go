package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"uuid"

	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

type ListNotificationsPageParams struct {
	Owner           string
	HasCursor       bool
	CursorCreatedAt time.Time
	CursorID        uuid.UUID
	Limit           int32
}

type ListNotificationsPageResult struct {
	Notifications []sqlcdb.Notification
	UnreadCount   int64
	HasMore       bool
}

func (s *SQLStore) ListNotificationsPage(
	ctx context.Context,
	arg ListNotificationsPageParams,
) (ListNotificationsPageResult, error) {
	var result ListNotificationsPageResult
	err := s.execTxOptions(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead}, func(q *sqlcdb.Queries) error {
		rows, err := q.ListNotifications(ctx, sqlcdb.ListNotificationsParams{
			Owner:           arg.Owner,
			HasCursor:       arg.HasCursor,
			CursorCreatedAt: arg.CursorCreatedAt,
			CursorID:        arg.CursorID,
			PageLimit:       arg.Limit + 1,
		})
		if err != nil {
			return ClassifyError(err)
		}
		if s.afterListNotifications != nil {
			s.afterListNotifications()
		}

		result.UnreadCount, err = q.CountUnreadNotifications(ctx, arg.Owner)
		if err != nil {
			return ClassifyError(err)
		}
		if len(rows) > int(arg.Limit) {
			result.HasMore = true
			rows = rows[:arg.Limit]
		}
		result.Notifications = rows
		return nil
	})
	return result, err
}

func (s *SQLStore) MarkNotificationReadTx(ctx context.Context, owner string, id uuid.UUID) (int64, error) {
	var unreadCount int64
	err := s.execTx(ctx, func(q *sqlcdb.Queries) error {
		if err := q.LockNotificationOwner(ctx, owner); err != nil {
			return ClassifyError(err)
		}
		if _, err := q.MarkNotificationRead(ctx, sqlcdb.MarkNotificationReadParams{
			ID:    id,
			Owner: owner,
		}); err != nil {
			return ClassifyError(err)
		}
		if s.afterMarkNotificationRead != nil {
			s.afterMarkNotificationRead()
		}

		var err error
		unreadCount, err = q.CountUnreadNotifications(ctx, owner)
		return ClassifyError(err)
	})
	return unreadCount, err
}

func (s *SQLStore) MarkAllNotificationsReadTx(ctx context.Context, owner string) (int64, error) {
	var unreadCount int64
	err := s.execTx(ctx, func(q *sqlcdb.Queries) error {
		if err := q.LockNotificationOwner(ctx, owner); err != nil {
			return ClassifyError(err)
		}
		if _, err := q.MarkAllNotificationsRead(ctx, owner); err != nil {
			return ClassifyError(err)
		}

		var err error
		unreadCount, err = q.CountUnreadNotifications(ctx, owner)
		return ClassifyError(err)
	})
	return unreadCount, err
}
