package config

import "testing"

func TestParseTransferLimits(t *testing.T) {
	t.Parallel()

	empty, err := parseTransferLimits("")
	if err != nil {
		t.Fatalf("empty input should not error, got %v", err)
	}
	if empty != nil {
		t.Errorf("empty input should yield no limits, got %v", empty)
	}

	limits, err := parseTransferLimits(`{"USD":{"max_per_transfer":100000,"daily":500000},"VND":{"max_per_transfer":2000000000}}`)
	if err != nil {
		t.Fatalf("valid JSON should parse, got %v", err)
	}
	if got := limits["USD"]; got.MaxPerTransfer != 100000 || got.Daily != 500000 {
		t.Errorf("USD limit = %+v, want {100000 500000}", got)
	}
	// VND omits daily, so it stays disabled (zero) while max applies.
	if got := limits["VND"]; got.MaxPerTransfer != 2000000000 || got.Daily != 0 {
		t.Errorf("VND limit = %+v, want {2000000000 0}", got)
	}

	if _, err := parseTransferLimits("not json"); err == nil {
		t.Error("malformed JSON should error")
	}

	if _, err := parseTransferLimits(
		`{"USD":{"max_per_transfer":9007199254740992}}`,
	); err == nil {
		t.Error("unsafe max_per_transfer should error")
	}
	if _, err := parseTransferLimits(
		`{"USD":{"daily":9007199254740992}}`,
	); err == nil {
		t.Error("unsafe daily limit should error")
	}
	if _, err := parseTransferLimits(
		`{"USD":{"max_per_transfer":-1}}`,
	); err == nil {
		t.Error("negative max_per_transfer should error")
	}
	if _, err := parseTransferLimits(
		`{"USD":{"daily":-1}}`,
	); err == nil {
		t.Error("negative daily limit should error")
	}
}

func TestValidateRejectsBadTransferLimits(t *testing.T) {
	t.Parallel()
	_, err := parseTransferLimits("{bad")
	cfg := Config{
		DBSource:          "x",
		JWTSecret:         "01234567890123456789012345678901",
		SMTPFrom:          "a@b.c",
		transferLimitsErr: err,
	}
	if cfg.Validate() == nil {
		t.Error("expected Validate to reject a transfer-limits parse error")
	}
}

func TestLimitFor(t *testing.T) {
	t.Parallel()
	cfg := Config{TransferLimits: map[string]CurrencyLimit{"USD": {MaxPerTransfer: 100000, Daily: 500000}}}
	if got := cfg.LimitFor("USD"); got.MaxPerTransfer != 100000 {
		t.Errorf("USD MaxPerTransfer = %d, want 100000", got.MaxPerTransfer)
	}
	// An unconfigured currency resolves to a zero-value (all limits disabled).
	if got := cfg.LimitFor("EUR"); got != (CurrencyLimit{}) {
		t.Errorf("unconfigured currency should disable limits, got %+v", got)
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	valid := Config{
		DBSource:            "postgres://u:p@localhost:5432/db",
		JWTSecret:           "01234567890123456789012345678901",
		SMTPFrom:            "no-reply@example.com",
		SessionCookieSecure: true,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}

	tests := []struct {
		name string
		cfg  Config
	}{
		{"missing db", Config{JWTSecret: "01234567890123456789012345678901", SMTPFrom: "a@b.c"}},
		{"short secret", Config{DBSource: "x", JWTSecret: "short", SMTPFrom: "a@b.c"}},
		{"31-char secret", Config{DBSource: "x", JWTSecret: "0123456789012345678901234567890", SMTPFrom: "a@b.c"}},
		{"missing from", Config{DBSource: "x", JWTSecret: "01234567890123456789012345678901"}},
		{"tls cert without key", Config{DBSource: "x", JWTSecret: "01234567890123456789012345678901", SMTPFrom: "a@b.c", TLSCertFile: "cert.pem"}},
		{"tls key without cert", Config{DBSource: "x", JWTSecret: "01234567890123456789012345678901", SMTPFrom: "a@b.c", TLSKeyFile: "key.pem"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.cfg.Validate(); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestValidateSessionCookieSecure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "https public url rejects insecure cookie",
			cfg: Config{
				DBSource:            "postgres://u:p@localhost:5432/db",
				JWTSecret:           "01234567890123456789012345678901",
				SMTPFrom:            "no-reply@example.com",
				PublicBaseURL:       "https://localhost:8443",
				SessionCookieSecure: false,
			},
			wantErr: true,
		},
		{
			name: "http public url allows insecure cookie",
			cfg: Config{
				DBSource:            "postgres://u:p@localhost:5432/db",
				JWTSecret:           "01234567890123456789012345678901",
				SMTPFrom:            "no-reply@example.com",
				PublicBaseURL:       "http://localhost:8080",
				SessionCookieSecure: false,
			},
		},
		{
			name: "scheme-less public url rejected",
			cfg: Config{
				DBSource:            "postgres://u:p@localhost:5432/db",
				JWTSecret:           "01234567890123456789012345678901",
				SMTPFrom:            "no-reply@example.com",
				PublicBaseURL:       "localhost:8080",
				SessionCookieSecure: true,
			},
			wantErr: true,
		},
		{
			name: "unsupported public url scheme rejected",
			cfg: Config{
				DBSource:            "postgres://u:p@localhost:5432/db",
				JWTSecret:           "01234567890123456789012345678901",
				SMTPFrom:            "no-reply@example.com",
				PublicBaseURL:       "ftp://localhost:8080",
				SessionCookieSecure: true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected valid config, got %v", err)
			}
		})
	}
}

func TestParseAccountOpeningLimits(t *testing.T) {
	t.Parallel()

	empty, err := parseAccountOpeningLimits("")
	if err != nil {
		t.Fatalf("empty input should not error, got %v", err)
	}
	if empty != nil {
		t.Errorf("empty input should yield no limits, got %v", empty)
	}

	limits, err := parseAccountOpeningLimits(`{"USD":100000,"EUR":100000,"VND":25000000}`)
	if err != nil {
		t.Fatalf("valid JSON should parse, got %v", err)
	}
	if got := limits["USD"]; got != 100000 {
		t.Errorf("USD limit = %d, want 100000", got)
	}
	if got := limits["VND"]; got != 25000000 {
		t.Errorf("VND limit = %d, want 25000000", got)
	}

	if _, err := parseAccountOpeningLimits("not json"); err == nil {
		t.Error("malformed JSON should error")
	}

	if _, err := parseAccountOpeningLimits(`{"USD":-100}`); err == nil {
		t.Error("negative cap should error")
	}

	if _, err := parseAccountOpeningLimits(
		`{"USD":9007199254740992}`,
	); err == nil {
		t.Error("unsafe opening cap should error")
	}
}

func TestValidateRejectsBadAccountOpeningLimits(t *testing.T) {
	t.Parallel()
	_, err := parseAccountOpeningLimits("{bad")
	cfg := Config{
		DBSource:                "x",
		JWTSecret:               "01234567890123456789012345678901",
		SMTPFrom:                "a@b.c",
		accountOpeningLimitsErr: err,
	}
	if cfg.Validate() == nil {
		t.Error("expected Validate to reject an account-opening-limits parse error")
	}
}

func TestOpeningBalanceLimitFor(t *testing.T) {
	t.Parallel()
	cfg := Config{AccountOpeningLimits: map[string]int64{"USD": 100000, "VND": 25000000}}
	if got := cfg.OpeningBalanceLimitFor("USD"); got != 100000 {
		t.Errorf("USD opening limit = %d, want 100000", got)
	}
	// An unconfigured currency resolves to zero (only zero opening allowed).
	if got := cfg.OpeningBalanceLimitFor("EUR"); got != 0 {
		t.Errorf("unconfigured currency should return 0, got %d", got)
	}
}
