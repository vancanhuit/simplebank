package config

import (
	"errors"
	"time"

	"github.com/urfave/cli/v3"
)

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
	RiverMaxWorkers int
	TLSCertFile     string
	TLSKeyFile      string
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
	return nil
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
		&cli.IntFlag{Name: "river-max-workers", Value: 10, Sources: cli.EnvVars("RIVER_MAX_WORKERS")},
		&cli.StringFlag{Name: "tls-cert-file", Sources: cli.EnvVars("TLS_CERT_FILE")},
		&cli.StringFlag{Name: "tls-key-file", Sources: cli.EnvVars("TLS_KEY_FILE")},
	}
}

func FromCommand(cmd *cli.Command) Config {
	return Config{
		HTTPAddr:        cmd.String("http-addr"),
		DBSource:        cmd.String("db-source"),
		DBMaxConns:      cmd.Int("db-max-conns"),
		DBMinConns:      cmd.Int("db-min-conns"),
		JWTSecret:       cmd.String("jwt-secret"),
		AccessTTL:       cmd.Duration("access-ttl"),
		RefreshTTL:      cmd.Duration("refresh-ttl"),
		SMTPHost:        cmd.String("smtp-host"),
		SMTPPort:        cmd.Int("smtp-port"),
		SMTPUsername:    cmd.String("smtp-username"),
		SMTPPassword:    cmd.String("smtp-password"),
		SMTPFrom:        cmd.String("smtp-from"),
		SMTPInsecure:    cmd.Bool("smtp-insecure"),
		RiverMaxWorkers: cmd.Int("river-max-workers"),
		TLSCertFile:     cmd.String("tls-cert-file"),
		TLSKeyFile:      cmd.String("tls-key-file"),
	}
}
