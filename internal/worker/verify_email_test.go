package worker

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"uuid"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

type fakeStore struct {
	store.Store
	getUser           func(context.Context, string) (sqlcdb.User, error)
	createVerifyEmail func(context.Context, sqlcdb.CreateVerifyEmailParams) (sqlcdb.VerifyEmail, error)
}

func (m fakeStore) GetUser(ctx context.Context, username string) (sqlcdb.User, error) {
	return m.getUser(ctx, username)
}

func (m fakeStore) CreateVerifyEmail(ctx context.Context, arg sqlcdb.CreateVerifyEmailParams) (sqlcdb.VerifyEmail, error) {
	return m.createVerifyEmail(ctx, arg)
}

type mockMailer struct {
	called           bool
	to, subject, msg string
	err              error
}

func (m *mockMailer) Send(_ context.Context, to, subject, htmlBody string) error {
	m.called = true
	m.to, m.subject, m.msg = to, subject, htmlBody
	return m.err
}

func TestSendVerifyEmailWorker(t *testing.T) {
	var verification sqlcdb.VerifyEmail
	var persistedCode string
	st := fakeStore{
		getUser: func(_ context.Context, username string) (sqlcdb.User, error) {
			return sqlcdb.User{Username: username, Email: "alice@example.com", FullName: "<b>Bob</b>"}, nil
		},
		createVerifyEmail: func(_ context.Context, arg sqlcdb.CreateVerifyEmailParams) (sqlcdb.VerifyEmail, error) {
			persistedCode = arg.SecretCode
			verification = sqlcdb.VerifyEmail{ID: uuid.New(), SecretCode: arg.SecretCode}
			return verification, nil
		},
	}
	mailer := &mockMailer{}
	w := NewSendVerifyEmailWorker(st, mailer, "https://bank.example.com")

	job := &river.Job[SendVerifyEmailArgs]{Args: SendVerifyEmailArgs{Username: "alice"}}
	if err := w.Work(t.Context(), job); err != nil {
		t.Fatalf("Work: %v", err)
	}

	if !mailer.called {
		t.Fatal("mailer should be invoked")
	}
	if mailer.to != "alice@example.com" {
		t.Errorf("recipient = %q, want alice@example.com", mailer.to)
	}
	// The full name is attacker-controlled; it must be HTML-escaped in the body.
	if !strings.Contains(mailer.msg, "&lt;b&gt;Bob&lt;/b&gt;") {
		t.Errorf("full name not HTML-escaped in body: %q", mailer.msg)
	}
	if strings.Contains(mailer.msg, "<b>Bob</b>") {
		t.Errorf("raw HTML from full name leaked into body: %q", mailer.msg)
	}
	_, hrefAndRest, ok := strings.Cut(mailer.msg, `href="`)
	if !ok {
		t.Fatalf("verification link missing from body: %q", mailer.msg)
	}
	href, _, ok := strings.Cut(hrefAndRest, `"`)
	if !ok {
		t.Fatalf("verification link is not terminated: %q", mailer.msg)
	}
	verificationURL, err := url.Parse(href)
	if err != nil {
		t.Fatalf("parse verification link: %v", err)
	}
	if verificationURL.Scheme != "https" || verificationURL.Host != "bank.example.com" || verificationURL.Path != "/verify-email" {
		t.Errorf("verification link target = %q, want https://bank.example.com/verify-email", verificationURL)
	}
	query := verificationURL.Query()
	if query.Get("id") != verification.ID.String() || query.Get("code") == "" {
		t.Errorf("verification link credentials = %v, want id=%s and a code", query, verification.ID)
	}
	if query.Get("code") == persistedCode {
		t.Fatal("verification bearer credential was persisted without hashing")
	}
}

func TestSendVerifyEmailWorkerErrors(t *testing.T) {
	getUserErr := errors.New("get user")
	createVerifyEmailErr := errors.New("create verify email")
	sendErr := errors.New("send email")
	user := sqlcdb.User{Username: "alice", Email: "alice@example.com"}

	tests := []struct {
		name    string
		store   fakeStore
		mailer  *mockMailer
		wantErr error
	}{
		{
			name: "get user",
			store: fakeStore{
				getUser: func(context.Context, string) (sqlcdb.User, error) {
					return sqlcdb.User{}, getUserErr
				},
			},
			mailer:  &mockMailer{},
			wantErr: getUserErr,
		},
		{
			name: "create verification record",
			store: fakeStore{
				getUser: func(context.Context, string) (sqlcdb.User, error) {
					return user, nil
				},
				createVerifyEmail: func(context.Context, sqlcdb.CreateVerifyEmailParams) (sqlcdb.VerifyEmail, error) {
					return sqlcdb.VerifyEmail{}, createVerifyEmailErr
				},
			},
			mailer:  &mockMailer{},
			wantErr: createVerifyEmailErr,
		},
		{
			name: "send email",
			store: fakeStore{
				getUser: func(context.Context, string) (sqlcdb.User, error) {
					return user, nil
				},
				createVerifyEmail: func(_ context.Context, arg sqlcdb.CreateVerifyEmailParams) (sqlcdb.VerifyEmail, error) {
					return sqlcdb.VerifyEmail{ID: uuid.New(), SecretCode: arg.SecretCode}, nil
				},
			},
			mailer:  &mockMailer{err: sendErr},
			wantErr: sendErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker := NewSendVerifyEmailWorker(tt.store, tt.mailer, "https://bank.example.com")
			job := &river.Job[SendVerifyEmailArgs]{Args: SendVerifyEmailArgs{Username: user.Username}}

			err := worker.Work(t.Context(), job)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Work error = %v, want wrapped %v", err, tt.wantErr)
			}
		})
	}
}

func TestSendVerifyEmailArgsInsertOpts(t *testing.T) {
	got := (SendVerifyEmailArgs{}).InsertOpts().UniqueOpts
	want := river.UniqueOpts{ByArgs: true, ByPeriod: 15 * time.Minute}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unique opts = %#v, want %#v", got, want)
	}
}
