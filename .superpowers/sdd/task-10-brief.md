# Task 10: Mailer interface and go-mail SMTP implementation

**Files:**
- Create: `internal/mail/mailer.go`
- Create: `internal/mail/smtp.go`

## Produces
- `type Mailer interface { Send(ctx context.Context, to, subject, htmlBody string) error }`
- `type SMTPMailer struct {...}`
- `func NewSMTPMailer(cfg config.Config) (*SMTPMailer, error)`

## Step 1: `internal/mail/mailer.go`
```go
package mail

import "context"

type Mailer interface {
	Send(ctx context.Context, to, subject, htmlBody string) error
}
```

## Step 2: `internal/mail/smtp.go`
```go
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
	opts := []mail.Option{
		mail.WithPort(cfg.SMTPPort),
		mail.WithTLSPolicy(mail.NoTLS),
	}
	if cfg.SMTPUsername != "" {
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(cfg.SMTPUsername),
			mail.WithPassword(cfg.SMTPPassword),
			mail.WithTLSPolicy(mail.TLSMandatory),
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
```

COMPATIBILITY — this is the critical part of the task. The exact go-mail API names may differ in the installed version. Verify each against the installed `github.com/wneessen/go-mail` and adjust to the exact exported identifiers so `go build ./internal/mail/` compiles:
- Client options: `mail.WithPort`, `mail.WithTLSPolicy`, TLS policy consts `mail.NoTLS` / `mail.TLSMandatory`, `mail.WithSMTPAuth`, auth const `mail.SMTPAuthPlain`, `mail.WithUsername`, `mail.WithPassword`.
- `mail.NewClient(host, opts...)`.
- Message: `mail.NewMsg()`, `msg.From`, `msg.To`, `msg.Subject`, `msg.SetBodyString`, content type const `mail.TypeTextHTML`.
- Send: `m.client.DialAndSendWithContext(ctx, msg)`.

If the installed version uses different names (e.g. `mail.WithTLSPortPolicy`, `mail.NoTLS` vs `mail.TLSOpportunistic`, or `SetBodyString` vs `SetBodyStringHTML`), use the installed version's actual identifiers. The REQUIRED BEHAVIOR is: build a client for `cfg.SMTPHost:cfg.SMTPPort`, use no TLS + no auth when username is empty (Mailpit local dev), use TLS + PLAIN auth when a username is provided, and send an HTML message from `cfg.SMTPFrom`. Keep the `Mailer` interface signature EXACTLY as specified regardless.

Keep `SMTPPort` as the type it is in `config.Config` (it's `int`); if `mail.WithPort` needs `int`, pass directly.

## Step 3: Verify build
`go build ./internal/mail/` and `go vet ./internal/mail/` → clean.

(No unit test in this task — SMTP sending is exercised via the worker/integration later. Do NOT add a networked test here.)

## Step 4: Commit
```bash
git add internal/mail/
git commit -m "feat: add generic SMTP mailer using go-mail"
```

## Global Constraints
- Generic SMTP (provider-agnostic); no hardcoded provider.
- Never log passwords.
- Conventional commit message.

## Report contract
Write full report to `.superpowers/sdd/task-10-report.md`, listing the EXACT go-mail identifiers you ended up using (option funcs, TLS policy const, auth const, body method, send method) and the installed go-mail version. Return only: status, commit hash(es), one-line build summary, and the go-mail identifiers/version.
