# Task 10 Report: Mailer interface and go-mail SMTP implementation

## Status
DONE

## Files created
- `internal/mail/mailer.go` — `Mailer` interface.
- `internal/mail/smtp.go` — `SMTPMailer` + `NewSMTPMailer`.

## go-mail version
`github.com/wneessen/go-mail v0.8.1` (const `VERSION = "0.8.1"`)

All template identifiers matched the installed version exactly; no substitutions were required. Verified via `go doc` and grep against the module cache at `~/go/pkg/mod/github.com/wneessen/go-mail@v0.8.1`.

## EXACT go-mail identifiers used
| Purpose | Identifier | Signature/Value (v0.8.1) |
|---|---|---|
| Port option | `mail.WithPort` | `func WithPort(port int) Option` (takes `int`, matches `config.Config.SMTPPort`) |
| TLS policy option | `mail.WithTLSPolicy` | `func WithTLSPolicy(policy TLSPolicy) Option` |
| TLS policy const (no TLS) | `mail.NoTLS` | `NoTLS TLSPolicy` (value 2) — forces unencrypted |
| TLS policy const (mandatory) | `mail.TLSMandatory` | `TLSMandatory TLSPolicy = iota` (value 0) — requires STARTTLS |
| SMTP auth option | `mail.WithSMTPAuth` | `func WithSMTPAuth(authtype SMTPAuthType) Option` |
| Auth const | `mail.SMTPAuthPlain` | `SMTPAuthPlain SMTPAuthType = "PLAIN"` |
| Username option | `mail.WithUsername` | `func WithUsername(username string) Option` |
| Password option | `mail.WithPassword` | `func WithPassword(password string) Option` |
| Client constructor | `mail.NewClient` | `func NewClient(host string, opts ...Option) (*Client, error)` |
| Message constructor | `mail.NewMsg` | `func NewMsg(opts ...MsgOption) *Msg` |
| From | `msg.From` | `func (m *Msg) From(from string) error` |
| To | `msg.To` | `func (m *Msg) To(rcpts ...string) error` |
| Subject | `msg.Subject` | `func (m *Msg) Subject(subj string)` |
| Body method | `msg.SetBodyString` | `func (m *Msg) SetBodyString(contentType ContentType, content string, opts ...PartOption)` |
| Content type const | `mail.TypeTextHTML` | `TypeTextHTML ContentType = "text/html"` |
| Send method | `client.DialAndSendWithContext` | `func (c *Client) DialAndSendWithContext(ctx context.Context, messages ...*Msg) (err error)` |

## Behavior
- Client is built for `cfg.SMTPHost:cfg.SMTPPort`.
- Username empty → `NoTLS`, no auth (Mailpit local dev).
- Username set → appends PLAIN auth + username + password + `TLSMandatory` (STARTTLS).
- `Send` builds an HTML message from `cfg.SMTPFrom` and dials+sends with the caller's context.
- Passwords are never logged.
- `Mailer` interface signature kept exactly: `Send(ctx context.Context, to, subject, htmlBody string) error`.

## Build verification
- `go build ./internal/mail/` → clean.
- `go build ./...` → clean.
- `go vet ./...` → clean.

## Notes
- `go-mail` was already present in `go.mod`/`go.sum` (as `// indirect`), so the package builds cleanly without touching module files. I intentionally did NOT commit a `go mod tidy` result: tidy stripped several dependencies (River, testify, echo-jwt, tidwall, etc.) that are pre-added for later SDD tasks but not yet imported. `go.mod`/`go.sum` were restored, and the commit contains only `internal/mail/`.
- No networked/unit test added (per brief).
