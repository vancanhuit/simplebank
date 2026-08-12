package worker

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/vancanhuit/simplebank/internal/mail"
)

const registrationNoticeBody = `A registration attempt used this email address. If this was you, sign in to your existing SimpleBank account. If not, no action is required.`

type SendRegistrationNoticeArgs struct {
	Email string `json:"email" river:"unique"`
}

func (SendRegistrationNoticeArgs) Kind() string { return "send_registration_notice" }

func (SendRegistrationNoticeArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{
		ByArgs:   true,
		ByPeriod: time.Hour,
	}}
}

type SendRegistrationNoticeWorker struct {
	river.WorkerDefaults[SendRegistrationNoticeArgs]
	mailer mail.Mailer
}

func NewSendRegistrationNoticeWorker(mailer mail.Mailer) *SendRegistrationNoticeWorker {
	return &SendRegistrationNoticeWorker{mailer: mailer}
}

func (w *SendRegistrationNoticeWorker) Work(ctx context.Context, job *river.Job[SendRegistrationNoticeArgs]) error {
	return w.mailer.Send(ctx, job.Args.Email, "SimpleBank registration attempt", registrationNoticeBody)
}
