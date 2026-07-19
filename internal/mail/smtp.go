package mail

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/wneessen/go-mail"

	"github.com/vancanhuit/simplebank/internal/config"
)

type SMTPMailer struct {
	client *mail.Client
	from   string
}

func NewSMTPMailer(cfg config.Config) (*SMTPMailer, error) {
	opts := []mail.Option{
		mail.WithPort(cfg.SMTPPort),
	}

	switch {
	case cfg.SMTPSSL:
		// Implicit TLS (SSL, typically port 465): the whole session is
		// encrypted from the first byte, so STARTTLS is not used.
		opts = append(opts, mail.WithSSL())
	case cfg.SMTPInsecure:
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	default:
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	}

	// Trust a specific CA (e.g. a mkcert root for local dev) instead of the
	// system roots when verifying the server certificate.
	if cfg.SMTPTLSCAFile != "" {
		pem, err := os.ReadFile(cfg.SMTPTLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("read smtp tls ca file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certificates found in %s", cfg.SMTPTLSCAFile)
		}
		opts = append(opts, mail.WithTLSConfig(&tls.Config{
			ServerName: cfg.SMTPHost,
			RootCAs:    pool,
			MinVersion: tls.VersionTLS12,
		}))
	}

	if cfg.SMTPUsername != "" {
		if cfg.SMTPInsecure && !cfg.SMTPSSL {
			// Refuse to send credentials over an unencrypted connection.
			return nil, fmt.Errorf("smtp auth requires TLS: set smtp-ssl or disable smtp-insecure")
		}
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(cfg.SMTPUsername),
			mail.WithPassword(cfg.SMTPPassword),
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
