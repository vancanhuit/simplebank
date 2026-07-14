package worker

import (
	"context"
	"fmt"
	"html"

	"github.com/riverqueue/river"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/mail"
	"github.com/vancanhuit/simplebank/internal/util"
)

type SendVerifyEmailArgs struct {
	Username string `json:"username"`
}

func (SendVerifyEmailArgs) Kind() string { return "send_verify_email" }

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
		return err
	}

	code, err := util.SecureToken(32)
	if err != nil {
		return err
	}

	ve, err := w.store.CreateVerifyEmail(ctx, sqlcdb.CreateVerifyEmailParams{
		Username:   user.Username,
		Email:      user.Email,
		SecretCode: code,
	})
	if err != nil {
		return err
	}

	link := fmt.Sprintf("%s/api/v1/users/verify_email?id=%s&code=%s",
		w.baseURL, ve.ID.String(), ve.SecretCode)
	body := fmt.Sprintf(
		`Hello %s,<br/>Please <a href="%s">click here</a> to verify your email address.`,
		html.EscapeString(user.FullName), link)

	return w.mailer.Send(ctx, user.Email, "Welcome to SimpleBank", body)
}
