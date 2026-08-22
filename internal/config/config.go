package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
	"github.com/vancanhuit/simplebank/internal/currency"
)

// CurrencyLimit holds the transfer ceilings for one currency, expressed in that
// currency's own minor units (e.g. USD/EUR cents, VND whole dong). A zero field
// disables that particular limit.
type CurrencyLimit struct {
	// MaxPerTransfer caps a single transfer.
	MaxPerTransfer int64 `json:"max_per_transfer"`
	// Daily caps the total outgoing amount per source account over a rolling
	// 24h window.
	Daily int64 `json:"daily"`
}

type Config struct {
	HTTPAddr            string
	DBSource            string
	DBMaxConns          int32
	DBMinConns          int32
	DBMaxConnLifetime   time.Duration
	DBMaxConnIdleTime   time.Duration
	JWTSecret           string
	AccessTTL           time.Duration
	RefreshTTL          time.Duration
	SMTPHost            string
	SMTPPort            int
	SMTPUsername        string
	SMTPPassword        string
	SMTPFrom            string
	SMTPInsecure        bool
	SMTPSSL             bool
	SMTPTLSCAFile       string
	RiverMaxWorkers     int
	TLSCertFile         string
	TLSKeyFile          string
	TrustedProxies      []string
	PublicBaseURL       string
	SessionCookieSecure bool
	// TransferLimits maps a currency code to its transfer ceilings. Because a
	// transfer's two accounts share one currency, each request resolves to a
	// single currency's limits. A currency absent from the map disables its
	// limits, so limits are opt-in per currency.
	TransferLimits    map[string]CurrencyLimit
	transferLimitsErr error
	// AccountOpeningLimits maps a currency code to the maximum opening balance
	// (in minor units) permitted when creating a new account. A missing
	// currency entry means zero: zero opening balance is allowed, positive is
	// rejected. This is a demo affordance; production banks fund accounts
	// through payment rails, not client-supplied opening balances.
	AccountOpeningLimits    map[string]int64
	accountOpeningLimitsErr error
}

// LimitFor returns the configured ceilings for a currency, or a zero-value
// (both limits disabled) when the currency has no entry.
func (c Config) LimitFor(currencyCode string) CurrencyLimit {
	return c.TransferLimits[currencyCode]
}

// OpeningBalanceLimitFor returns the configured cap for opening balances in a
// given currency. A missing currency entry returns zero, meaning only zero
// opening balance is allowed for that currency.
func (c Config) OpeningBalanceLimitFor(currencyCode string) int64 {
	return c.AccountOpeningLimits[currencyCode]
}

func (c Config) Validate() error {
	if c.DBSource == "" {
		return errors.New("db-source is required")
	}
	if len(c.JWTSecret) < 32 {
		return errors.New("jwt-secret must be at least 32 characters")
	}
	if c.SMTPFrom == "" {
		return errors.New("smtp-from is required")
	}
	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		return errors.New("tls-cert-file and tls-key-file must be set together")
	}
	if c.PublicBaseURL != "" {
		baseURL, err := url.Parse(c.PublicBaseURL)
		if err != nil {
			return fmt.Errorf("invalid public-base-url: %w", err)
		}
		if !strings.EqualFold(baseURL.Scheme, "http") && !strings.EqualFold(baseURL.Scheme, "https") {
			return errors.New("public-base-url must use an explicit http or https scheme")
		}
		if strings.EqualFold(baseURL.Scheme, "https") && !c.SessionCookieSecure {
			return errors.New("session-cookie-secure must be true for an HTTPS public-base-url")
		}
	}
	if c.transferLimitsErr != nil {
		return fmt.Errorf("invalid transfer-limits: %w", c.transferLimitsErr)
	}
	if c.accountOpeningLimitsErr != nil {
		return fmt.Errorf("invalid account-opening-limits: %w", c.accountOpeningLimitsErr)
	}
	return nil
}

// parseTransferLimits decodes the transfer-limits JSON object. An empty value
// yields no limits; malformed JSON is returned as an error surfaced by Validate.
func parseTransferLimits(raw string) (map[string]CurrencyLimit, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var limits map[string]CurrencyLimit
	if err := json.Unmarshal([]byte(raw), &limits); err != nil {
		return nil, err
	}
	for code, limit := range limits {
		if limit.MaxPerTransfer < 0 {
			return nil, fmt.Errorf("max per-transfer limit for %s must not be negative", code)
		}
		if limit.Daily < 0 {
			return nil, fmt.Errorf("daily limit for %s must not be negative", code)
		}
		if limit.MaxPerTransfer > currency.MaxSafeMinorUnits {
			return nil, fmt.Errorf("max per-transfer limit for %s exceeds JavaScript safe integer", code)
		}
		if limit.Daily > currency.MaxSafeMinorUnits {
			return nil, fmt.Errorf("daily limit for %s exceeds JavaScript safe integer", code)
		}
	}
	return limits, nil
}

// parseAccountOpeningLimits decodes the account-opening-limits JSON object. An
// empty value yields no limits; malformed JSON or negative caps are returned as
// errors surfaced by Validate.
func parseAccountOpeningLimits(raw string) (map[string]int64, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var limits map[string]int64
	if err := json.Unmarshal([]byte(raw), &limits); err != nil {
		return nil, err
	}
	// Reject any negative cap.
	for currencyCode, cap := range limits {
		if cap < 0 {
			return nil, fmt.Errorf("negative opening balance cap for %s: %d", currencyCode, cap)
		}
		if cap > currency.MaxSafeMinorUnits {
			return nil, fmt.Errorf("opening balance cap for %s exceeds JavaScript safe integer", currencyCode)
		}
	}
	return limits, nil
}

func Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "http-addr", Value: ":8080", Sources: cli.EnvVars("HTTP_ADDR")},
		&cli.StringFlag{Name: "db-source", Sources: cli.EnvVars("DB_SOURCE")},
		&cli.Int32Flag{Name: "db-max-conns", Sources: cli.EnvVars("DB_MAX_CONNS")},
		&cli.Int32Flag{Name: "db-min-conns", Sources: cli.EnvVars("DB_MIN_CONNS")},
		&cli.DurationFlag{Name: "db-max-conn-lifetime", Sources: cli.EnvVars("DB_MAX_CONN_LIFETIME")},
		&cli.DurationFlag{Name: "db-max-conn-idle-time", Sources: cli.EnvVars("DB_MAX_CONN_IDLE_TIME")},
		&cli.StringFlag{Name: "jwt-secret", Sources: cli.EnvVars("JWT_SECRET")},
		&cli.DurationFlag{Name: "access-ttl", Value: 15 * time.Minute, Sources: cli.EnvVars("ACCESS_TTL")},
		&cli.DurationFlag{Name: "refresh-ttl", Value: 24 * time.Hour, Sources: cli.EnvVars("REFRESH_TTL")},
		&cli.StringFlag{Name: "smtp-host", Sources: cli.EnvVars("SMTP_HOST")},
		&cli.IntFlag{Name: "smtp-port", Value: 1025, Sources: cli.EnvVars("SMTP_PORT")},
		&cli.StringFlag{Name: "smtp-username", Sources: cli.EnvVars("SMTP_USERNAME")},
		&cli.StringFlag{Name: "smtp-password", Sources: cli.EnvVars("SMTP_PASSWORD")},
		&cli.StringFlag{Name: "smtp-from", Sources: cli.EnvVars("SMTP_FROM")},
		&cli.BoolFlag{Name: "smtp-insecure", Sources: cli.EnvVars("SMTP_INSECURE")},
		&cli.BoolFlag{Name: "smtp-ssl", Sources: cli.EnvVars("SMTP_SSL")},
		&cli.StringFlag{Name: "smtp-tls-ca-file", Sources: cli.EnvVars("SMTP_TLS_CA_FILE")},
		&cli.IntFlag{Name: "river-max-workers", Value: 10, Sources: cli.EnvVars("RIVER_MAX_WORKERS")},
		&cli.StringFlag{Name: "tls-cert-file", Sources: cli.EnvVars("TLS_CERT_FILE")},
		&cli.StringFlag{Name: "tls-key-file", Sources: cli.EnvVars("TLS_KEY_FILE")},
		&cli.StringSliceFlag{Name: "trusted-proxies", Sources: cli.EnvVars("TRUSTED_PROXIES")},
		&cli.StringFlag{Name: "public-base-url", Sources: cli.EnvVars("PUBLIC_BASE_URL")},
		&cli.BoolFlag{Name: "session-cookie-secure", Value: true, Sources: cli.EnvVars("SESSION_COOKIE_SECURE")},
		&cli.StringFlag{Name: "transfer-limits", Sources: cli.EnvVars("TRANSFER_LIMITS")},
		&cli.StringFlag{Name: "account-opening-limits", Sources: cli.EnvVars("ACCOUNT_OPENING_LIMITS")},
	}
}

func FromCommand(cmd *cli.Command) Config {
	limits, limitsErr := parseTransferLimits(cmd.String("transfer-limits"))
	openingLimits, openingLimitsErr := parseAccountOpeningLimits(cmd.String("account-opening-limits"))
	return Config{
		HTTPAddr:                cmd.String("http-addr"),
		DBSource:                cmd.String("db-source"),
		DBMaxConns:              cmd.Int32("db-max-conns"),
		DBMinConns:              cmd.Int32("db-min-conns"),
		DBMaxConnLifetime:       cmd.Duration("db-max-conn-lifetime"),
		DBMaxConnIdleTime:       cmd.Duration("db-max-conn-idle-time"),
		JWTSecret:               cmd.String("jwt-secret"),
		AccessTTL:               cmd.Duration("access-ttl"),
		RefreshTTL:              cmd.Duration("refresh-ttl"),
		SMTPHost:                cmd.String("smtp-host"),
		SMTPPort:                cmd.Int("smtp-port"),
		SMTPUsername:            cmd.String("smtp-username"),
		SMTPPassword:            cmd.String("smtp-password"),
		SMTPFrom:                cmd.String("smtp-from"),
		SMTPInsecure:            cmd.Bool("smtp-insecure"),
		SMTPSSL:                 cmd.Bool("smtp-ssl"),
		SMTPTLSCAFile:           cmd.String("smtp-tls-ca-file"),
		RiverMaxWorkers:         cmd.Int("river-max-workers"),
		TLSCertFile:             cmd.String("tls-cert-file"),
		TLSKeyFile:              cmd.String("tls-key-file"),
		TrustedProxies:          cmd.StringSlice("trusted-proxies"),
		PublicBaseURL:           cmd.String("public-base-url"),
		SessionCookieSecure:     cmd.Bool("session-cookie-secure"),
		TransferLimits:          limits,
		AccountOpeningLimits:    openingLimits,
		accountOpeningLimitsErr: openingLimitsErr,
		transferLimitsErr:       limitsErr,
	}
}
