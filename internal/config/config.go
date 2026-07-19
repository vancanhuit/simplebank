package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
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
	HTTPAddr        string
	DBSource        string
	DBMaxConns      int
	DBMinConns      int
	JWTSecret       string
	AccessTTL       time.Duration
	RefreshTTL      time.Duration
	SMTPHost        string
	SMTPPort        int
	SMTPUsername    string
	SMTPPassword    string
	SMTPFrom        string
	SMTPInsecure    bool
	SMTPSSL         bool
	SMTPTLSCAFile   string
	RiverMaxWorkers int
	TLSCertFile     string
	TLSKeyFile      string
	TrustedProxies  []string
	PublicBaseURL   string
	// TransferLimits maps a currency code to its transfer ceilings. Because a
	// transfer's two accounts share one currency, each request resolves to a
	// single currency's limits. A currency absent from the map disables its
	// limits, so limits are opt-in per currency.
	TransferLimits    map[string]CurrencyLimit
	transferLimitsErr error
}

// LimitFor returns the configured ceilings for a currency, or a zero-value
// (both limits disabled) when the currency has no entry.
func (c Config) LimitFor(currencyCode string) CurrencyLimit {
	return c.TransferLimits[currencyCode]
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
	if c.transferLimitsErr != nil {
		return fmt.Errorf("invalid transfer-limits: %w", c.transferLimitsErr)
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
	return limits, nil
}

func Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "http-addr", Value: ":8080", Sources: cli.EnvVars("HTTP_ADDR")},
		&cli.StringFlag{Name: "db-source", Sources: cli.EnvVars("DB_SOURCE")},
		&cli.IntFlag{Name: "db-max-conns", Sources: cli.EnvVars("DB_MAX_CONNS")},
		&cli.IntFlag{Name: "db-min-conns", Sources: cli.EnvVars("DB_MIN_CONNS")},
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
		&cli.StringFlag{Name: "transfer-limits", Sources: cli.EnvVars("TRANSFER_LIMITS")},
	}
}

func FromCommand(cmd *cli.Command) Config {
	limits, limitsErr := parseTransferLimits(cmd.String("transfer-limits"))
	return Config{
		HTTPAddr:          cmd.String("http-addr"),
		DBSource:          cmd.String("db-source"),
		DBMaxConns:        cmd.Int("db-max-conns"),
		DBMinConns:        cmd.Int("db-min-conns"),
		JWTSecret:         cmd.String("jwt-secret"),
		AccessTTL:         cmd.Duration("access-ttl"),
		RefreshTTL:        cmd.Duration("refresh-ttl"),
		SMTPHost:          cmd.String("smtp-host"),
		SMTPPort:          cmd.Int("smtp-port"),
		SMTPUsername:      cmd.String("smtp-username"),
		SMTPPassword:      cmd.String("smtp-password"),
		SMTPFrom:          cmd.String("smtp-from"),
		SMTPInsecure:      cmd.Bool("smtp-insecure"),
		SMTPSSL:           cmd.Bool("smtp-ssl"),
		SMTPTLSCAFile:     cmd.String("smtp-tls-ca-file"),
		RiverMaxWorkers:   cmd.Int("river-max-workers"),
		TLSCertFile:       cmd.String("tls-cert-file"),
		TLSKeyFile:        cmd.String("tls-key-file"),
		TrustedProxies:    cmd.StringSlice("trusted-proxies"),
		PublicBaseURL:     cmd.String("public-base-url"),
		TransferLimits:    limits,
		transferLimitsErr: limitsErr,
	}
}
