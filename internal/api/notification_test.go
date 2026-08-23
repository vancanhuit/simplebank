package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"uuid"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

func TestNotificationEndpointsRequireAuthentication(t *testing.T) {
	t.Parallel()

	s := newTestServerWithStore(t, fakeStore{})
	for _, endpoint := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/notifications"},
		{method: http.MethodPut, path: "/api/v1/notifications/" + uuid.New().String() + "/read"},
		{method: http.MethodPut, path: "/api/v1/notifications/read-all"},
	} {
		req := httptest.NewRequest(endpoint.method, endpoint.path, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401", endpoint.method, endpoint.path, rec.Code)
		}
	}
}

func TestListNotificationsUsesAuthenticatedOwner(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, time.August, 23, 10, 30, 0, 0, time.UTC)
	readAt := createdAt.Add(time.Minute)
	notification := sqlcdb.Notification{
		ID:         uuid.MustParse("0198d94d-9380-7d00-8000-000000000001"),
		Owner:      "alice",
		AccountID:  uuid.MustParse("0198d94d-9380-7d00-8000-000000000002"),
		TransferID: uuid.MustParse("0198d94d-9380-7d00-8000-000000000003"),
		Direction:  "sent",
		Amount:     1250,
		Currency:   "USD",
		Balance:    8750,
		ReadAt:     pgtype.Timestamptz{Time: readAt, Valid: true},
		CreatedAt:  createdAt,
	}
	fake := fakeStore{
		listNotificationsPage: func(_ context.Context, arg store.ListNotificationsPageParams) (store.ListNotificationsPageResult, error) {
			if arg.Owner != "alice" {
				t.Fatalf("owner = %q, want authenticated owner alice", arg.Owner)
			}
			if arg.Limit != 20 || arg.HasCursor {
				t.Fatalf("pagination = %+v, want default limit without cursor", arg)
			}
			return store.ListNotificationsPageResult{
				Notifications: []sqlcdb.Notification{notification},
				UnreadCount:   4,
			}, nil
		},
	}
	s := newTestServerWithStore(t, fake)
	tokens := mustIssueTokenPair(t, "alice")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
	req.Header.Set("Authorization", "Bearer "+tokens.access)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Notifications []map[string]any `json:"notifications"`
		UnreadCount   int64            `json:"unread_count"`
		NextCursor    *string          `json:"next_cursor"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.UnreadCount != 4 || got.NextCursor != nil || len(got.Notifications) != 1 {
		t.Fatalf("response = %+v", got)
	}
	row := got.Notifications[0]
	if _, exposed := row["owner"]; exposed {
		t.Fatal("notification response exposed owner")
	}
	if row["id"] != notification.ID.String() || row["account_id"] != notification.AccountID.String() ||
		row["transfer_id"] != notification.TransferID.String() || row["direction"] != "sent" ||
		row["amount"] != float64(1250) || row["currency"] != "USD" || row["balance"] != float64(8750) ||
		row["read_at"] != readAt.Format(time.RFC3339) || row["created_at"] != createdAt.Format(time.RFC3339) {
		t.Fatalf("notification response = %#v", row)
	}
}

func TestListNotificationsNormalizesSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		query     string
		wantLimit int32
		wantCode  int
	}{
		{name: "below minimum uses default", query: "?size=0", wantLimit: 20, wantCode: http.StatusOK},
		{name: "above maximum clamps", query: "?size=101", wantLimit: 100, wantCode: http.StatusOK},
		{name: "non numeric rejected", query: "?size=many", wantCode: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := fakeStore{
				listNotificationsPage: func(_ context.Context, arg store.ListNotificationsPageParams) (store.ListNotificationsPageResult, error) {
					if arg.Owner != "alice" || arg.Limit != tt.wantLimit {
						t.Fatalf("list args = %+v, want owner alice and limit %d", arg, tt.wantLimit)
					}
					return store.ListNotificationsPageResult{}, nil
				},
			}
			s := newTestServerWithStore(t, fake)
			tokens := mustIssueTokenPair(t, "alice")
			req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications"+tt.query, nil)
			req.Header.Set("Authorization", "Bearer "+tokens.access)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.wantCode, rec.Body.String())
			}
		})
	}
}

func TestListNotificationsRejectsInvalidCursor(t *testing.T) {
	t.Parallel()

	encode := func(raw string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(raw))
	}
	nonCanonical := func(raw string) string {
		for len(raw)%3 == 0 {
			raw += " "
		}
		encoded := encode(raw)
		const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
		last := strings.IndexByte(alphabet, encoded[len(encoded)-1])
		return encoded[:len(encoded)-1] + string(alphabet[last|1])
	}
	tests := []struct {
		name   string
		cursor string
	}{
		{name: "empty", cursor: ""},
		{name: "invalid base64url", cursor: "***"},
		{name: "unknown field", cursor: encode(`{"created_at":"2026-08-23T10:30:00Z","id":"0198d94d-9380-7d00-8000-000000000001","owner":"alice"}`)},
		{name: "trailing JSON", cursor: encode(`{"created_at":"2026-08-23T10:30:00Z","id":"0198d94d-9380-7d00-8000-000000000001"}{}`)},
		{name: "zero time", cursor: encode(`{"created_at":"0001-01-01T00:00:00Z","id":"0198d94d-9380-7d00-8000-000000000001"}`)},
		{name: "nil UUID", cursor: encode(`{"created_at":"2026-08-23T10:30:00Z","id":"00000000-0000-0000-0000-000000000000"}`)},
		{name: "padded base64", cursor: encode(`{"created_at":"2026-08-23T10:30:00Z","id":"0198d94d-9380-7d00-8000-000000000001"}`) + "="},
		{name: "non canonical base64url", cursor: nonCanonical(`{"created_at":"2026-08-23T10:30:00Z","id":"0198d94d-9380-7d00-8000-000000000001"}`)},
		{name: "duplicate query value", cursor: encode(`{"created_at":"2026-08-23T10:30:00Z","id":"0198d94d-9380-7d00-8000-000000000001"}`) + "&cursor=ignored"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := fakeStore{
				listNotificationsPage: func(context.Context, store.ListNotificationsPageParams) (store.ListNotificationsPageResult, error) {
					t.Fatal("invalid cursor must be rejected before store access")
					return store.ListNotificationsPageResult{}, nil
				},
			}
			s := newTestServerWithStore(t, fake)
			tokens := mustIssueTokenPair(t, "alice")
			req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?cursor="+tt.cursor, nil)
			req.Header.Set("Authorization", "Bearer "+tokens.access)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest || rec.Body.String() != "{\"error\":\"invalid notification cursor\"}\n" {
				t.Fatalf("response = %d %q, want cursor 400", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestListNotificationsReturnsStableNextCursor(t *testing.T) {
	t.Parallel()

	firstTime := time.Date(2026, time.August, 23, 10, 31, 0, 0, time.UTC)
	lastTime := time.Date(2026, time.August, 23, 10, 30, 0, 0, time.UTC)
	firstID := uuid.MustParse("0198d94d-9380-7d00-8000-000000000003")
	lastID := uuid.MustParse("0198d94d-9380-7d00-8000-000000000002")
	fake := fakeStore{
		listNotificationsPage: func(_ context.Context, arg store.ListNotificationsPageParams) (store.ListNotificationsPageResult, error) {
			if arg.Owner != "alice" || !arg.HasCursor || !arg.CursorCreatedAt.Equal(firstTime) || arg.CursorID != firstID {
				t.Fatalf("cursor args = %+v", arg)
			}
			return store.ListNotificationsPageResult{
				Notifications: []sqlcdb.Notification{
					{ID: firstID, CreatedAt: firstTime},
					{ID: lastID, CreatedAt: lastTime},
				},
				HasMore: true,
			}, nil
		},
	}
	s := newTestServerWithStore(t, fake)
	tokens := mustIssueTokenPair(t, "alice")
	incoming := base64.RawURLEncoding.EncodeToString([]byte(`{"created_at":"2026-08-23T10:31:00Z","id":"0198d94d-9380-7d00-8000-000000000003"}`))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?size=2&cursor="+incoming, nil)
	req.Header.Set("Authorization", "Bearer "+tokens.access)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		NextCursor *string `json:"next_cursor"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	want := base64.RawURLEncoding.EncodeToString([]byte(`{"created_at":"2026-08-23T10:30:00Z","id":"0198d94d-9380-7d00-8000-000000000002"}`))
	if got.NextCursor == nil || *got.NextCursor != want {
		t.Fatalf("next_cursor = %v, want %q", got.NextCursor, want)
	}
}

func TestMarkNotificationReadUsesAuthenticatedOwner(t *testing.T) {
	t.Parallel()

	id := uuid.MustParse("0198d94d-9380-7d00-8000-000000000001")
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		fake := fakeStore{
			markNotificationReadTx: func(_ context.Context, owner string, gotID uuid.UUID) (int64, error) {
				if owner != "alice" || gotID != id {
					t.Fatalf("mark read args = %q, %s", owner, gotID)
				}
				return 3, nil
			},
		}
		rec := authenticatedNotificationRequest(t, newTestServerWithStore(t, fake), http.MethodPut, "/api/v1/notifications/"+id.String()+"/read")
		if rec.Code != http.StatusOK || rec.Body.String() != "{\"unread_count\":3}\n" {
			t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
		}
	})

	t.Run("foreign notification is indistinguishable from missing", func(t *testing.T) {
		t.Parallel()
		fake := fakeStore{
			markNotificationReadTx: func(_ context.Context, owner string, gotID uuid.UUID) (int64, error) {
				if owner != "alice" || gotID != id {
					t.Fatalf("mark read args = %q, %s", owner, gotID)
				}
				return 0, store.ErrRecordNotFound
			},
		}
		rec := authenticatedNotificationRequest(t, newTestServerWithStore(t, fake), http.MethodPut, "/api/v1/notifications/"+id.String()+"/read")
		if rec.Code != http.StatusNotFound || rec.Body.String() != "{\"error\":\"resource not found\"}\n" {
			t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
		}
	})

	t.Run("malformed id", func(t *testing.T) {
		t.Parallel()
		rec := authenticatedNotificationRequest(t, newTestServerWithStore(t, fakeStore{}), http.MethodPut, "/api/v1/notifications/not-a-uuid/read")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
		}
	})
}

func TestMarkAllNotificationsReadUsesAuthenticatedOwner(t *testing.T) {
	t.Parallel()

	fake := fakeStore{
		markAllNotificationsReadTx: func(_ context.Context, owner string) (int64, error) {
			if owner != "alice" {
				t.Fatalf("owner = %q, want authenticated owner alice", owner)
			}
			return 0, nil
		},
	}
	rec := authenticatedNotificationRequest(t, newTestServerWithStore(t, fake), http.MethodPut, "/api/v1/notifications/read-all")
	if rec.Code != http.StatusOK || rec.Body.String() != "{\"unread_count\":0}\n" {
		t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
	}
}

func authenticatedNotificationRequest(t *testing.T, s *Server, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	tokens := mustIssueTokenPair(t, "alice")
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+tokens.access)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestListNotificationsWithoutMoreRowsReturnsNoCursor(t *testing.T) {
	t.Parallel()

	fake := fakeStore{
		listNotificationsPage: func(context.Context, store.ListNotificationsPageParams) (store.ListNotificationsPageResult, error) {
			return store.ListNotificationsPageResult{
				Notifications: []sqlcdb.Notification{{ID: uuid.New(), CreatedAt: time.Now()}},
				HasMore:       false,
			}, nil
		},
	}
	rec := authenticatedNotificationRequest(t, newTestServerWithStore(t, fake), http.MethodGet, "/api/v1/notifications")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"next_cursor":null`) {
		t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
	}
}
