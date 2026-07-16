package mail

import (
	"context"

	"github.com/wneessen/go-mail"

	"github.com/vancanhuit/simplebank/internal/config"
)

type SMTPMailer struct {
	client *mail.Client
	from   string
}

func NewSMTPMailer(cfg config.Config) (*SMTPMailer, error) {
	tlsPolicy := mail.TLSMandatory
	if cfg.SMTPInsecure {
		tlsPolicy = mail.NoTLS
	}
	opts := []mail.Option{
		mail.WithPort(cfg.SMTPPort),
		mail.WithTLSPolicy(tlsPolicy),
	}
	if cfg.SMTPUsername != "" {
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(cfg.SMTPUsername),
			mail.WithPassword(cfg.SMTPPassword),
			mail.WithTLSPolicy(mail.TLSMandatory), // never send credentials in cleartext
		)
	}
	client, err := mail.NewClient(cfg.SMTPHost, opts...)
	if err != nil {
		return nil, err
	}
	return &SMTPMailer{client: client, from: cfg.SMTPFrom}, nil
}

func (m *SMTPMailer) Send(ctx context.Context, to, subject, htmlBody string) error {
	msg := mail.NewMsg()
	if err := msg.From(m.from); err != nil {
		return err
	}
	if err := msg.To(to); err != nil {
		return err
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextHTML, htmlBody)
	return m.client.DialAndSendWithContext(ctx, msg)
}
