package worker

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/mail"
)

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return err
	}
	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	return err
}

func NewClient(
	pool *pgxpool.Pool,
	maxWorkers int,
	st store.Store,
	mailer mail.Mailer,
	baseURL string,
) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, NewSendVerifyEmailWorker(st, mailer, baseURL))
	river.AddWorker(workers, NewSendRegistrationNoticeWorker(mailer))

	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: maxWorkers},
		},
		Workers: workers,
	})
}
