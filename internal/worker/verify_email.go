package worker

import (
	"context"
	"fmt"
	"html"
	"time"

	"github.com/riverqueue/river"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/mail"
	"github.com/vancanhuit/simplebank/internal/secret"
)

type SendVerifyEmailArgs struct {
	Username string `json:"username" river:"unique"`
}

func (SendVerifyEmailArgs) Kind() string { return "send_verify_email" }

func (SendVerifyEmailArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{
		ByArgs:   true,
		ByPeriod: 15 * time.Minute,
	}}
}

type SendVerifyEmailWorker struct {
	river.WorkerDefaults[SendVerifyEmailArgs]
	store   store.Store
	mailer  mail.Mailer
	baseURL string
}

func NewSendVerifyEmailWorker(st store.Store, mailer mail.Mailer, baseURL string) *SendVerifyEmailWorker {
	return &SendVerifyEmailWorker{store: st, mailer: mailer, baseURL: baseURL}
}

func (w *SendVerifyEmailWorker) Work(ctx context.Context, job *river.Job[SendVerifyEmailArgs]) error {
	user, err := w.store.GetUser(ctx, job.Args.Username)
	if err != nil {
		return fmt.Errorf("getting user %q: %w", job.Args.Username, err)
	}

	code, err := secret.Token(32)
	if err != nil {
		return fmt.Errorf("generating secret code: %w", err)
	}

	ve, err := w.store.CreateVerifyEmail(ctx, sqlcdb.CreateVerifyEmailParams{
		Username:   user.Username,
		Email:      user.Email,
		SecretCode: code,
	})
	if err != nil {
		return fmt.Errorf("creating verify-email record: %w", err)
	}

	// Link to the SPA verification page (not the JSON API directly) so the user
	// lands on a rendered success/error screen. The page calls the API in turn.
	link := fmt.Sprintf("%s/verify-email?id=%s&code=%s",
		w.baseURL, ve.ID.String(), ve.SecretCode)
	body := fmt.Sprintf(
		`Hello %s,<br/>Please <a href="%s">click here</a> to verify your email address.`,
		html.EscapeString(user.FullName), link)

	if err := w.mailer.Send(ctx, user.Email, "Welcome to SimpleBank", body); err != nil {
		return fmt.Errorf("sending verification email: %w", err)
	}
	return nil
}
