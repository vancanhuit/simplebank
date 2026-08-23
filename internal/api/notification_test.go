package api

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	guuid "github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"uuid"

	"github.com/vancanhuit/simplebank/internal/config"
	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/notification"
	"github.com/vancanhuit/simplebank/internal/token"
)

func TestNotificationStreamRequiresAuthentication(t *testing.T) {
	s, _, _ := newNotificationStreamServer(t, time.Hour)
	testServer := httptest.NewServer(s.Handler())
	t.Cleanup(testServer.Close)

	resp, err := testServer.Client().Get(testServer.URL + "/api/v1/notifications/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestNotificationStreamFiltersAuthenticatedOwner(t *testing.T) {
	s, maker, hub := newNotificationStreamServer(t, time.Hour)
	testServer := httptest.NewServer(s.Handler())
	t.Cleanup(testServer.Close)

	resp, cancel := openNotificationStream(t, testServer, maker, "alice", time.Minute)
	defer cancel()
	defer func() { _ = resp.Body.Close() }()
	assertNotificationStreamHeaders(t, resp)
	reader := bufio.NewReader(resp.Body)
	if got := readSSEFrame(t, reader); got != ": connected\n\n" {
		t.Fatalf("initial frame = %q, want connected comment", got)
	}

	hub.Publish("bob", guuid.MustParse("0198d94d-9380-7d00-8000-000000000001"))
	aliceID := guuid.MustParse("0198d94d-9380-7d00-8000-000000000002")
	hub.Publish("alice", aliceID)
	if got, want := readSSEFrame(t, reader), "event: notification\ndata: "+aliceID.String()+"\n\n"; got != want {
		t.Fatalf("notification frame = %q, want %q", got, want)
	}
}

func TestNotificationStreamEmitsKeepalive(t *testing.T) {
	s, maker, _ := newNotificationStreamServer(t, 10*time.Millisecond)
	testServer := httptest.NewServer(s.Handler())
	t.Cleanup(testServer.Close)

	resp, cancel := openNotificationStream(t, testServer, maker, "alice", time.Minute)
	defer cancel()
	defer func() { _ = resp.Body.Close() }()
	reader := bufio.NewReader(resp.Body)
	if got := readSSEFrame(t, reader); got != ": connected\n\n" {
		t.Fatalf("initial frame = %q, want connected comment", got)
	}
	if got := readSSEFrame(t, reader); got != ": keepalive\n\n" {
		t.Fatalf("keepalive frame = %q, want keepalive comment", got)
	}
}

func TestNotificationStreamStopsAtTokenExpiry(t *testing.T) {
	s, maker, _ := newNotificationStreamServer(t, time.Hour)
	testServer := httptest.NewServer(s.Handler())
	t.Cleanup(testServer.Close)

	resp, cancel := openNotificationStream(t, testServer, maker, "alice", 1500*time.Millisecond)
	defer cancel()
	defer func() { _ = resp.Body.Close() }()
	reader := bufio.NewReader(resp.Body)
	if got := readSSEFrame(t, reader); got != ": connected\n\n" {
		t.Fatalf("initial frame = %q, want connected comment", got)
	}

	expired := make(chan error, 1)
	go func() {
		_, err := reader.ReadString('\n')
		expired <- err
	}()
	select {
	case err := <-expired:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("stream expiry error = %v, want EOF", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stream remained open after access token expiry")
	}
}

func TestNotificationStreamUnsubscribesOnDisconnect(t *testing.T) {
	s, maker, _ := newNotificationStreamServer(t, time.Hour)
	testServer := httptest.NewServer(s.Handler())

	resp, cancel := openNotificationStream(t, testServer, maker, "alice", time.Minute)
	reader := bufio.NewReader(resp.Body)
	if got := readSSEFrame(t, reader); got != ": connected\n\n" {
		t.Fatalf("initial frame = %q, want connected comment", got)
	}
	cancel()
	_ = resp.Body.Close()

	closed := make(chan struct{})
	go func() {
		testServer.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("stream handler did not stop and clean up after disconnect")
	}
}

type fakeDeadlineResponseWriter struct {
	*httptest.ResponseRecorder
	err error
}

func (writer *fakeDeadlineResponseWriter) SetWriteDeadline(time.Time) error {
	return writer.err
}

func TestNotificationStreamWriteDeadlineFailureUnsubscribes(t *testing.T) {
	s, _, _ := newNotificationStreamServer(t, time.Hour)
	unsubscribed := false
	s.subscribeNotifications = func(string) (<-chan guuid.UUID, func()) {
		return make(chan guuid.UUID), func() { unsubscribed = true }
	}
	tokens := mustIssueTokenPair(t, "alice")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/stream", nil)
	req.Header.Set("Authorization", "Bearer "+tokens.access)
	deadlineErr := errors.New("setting write deadline")
	writer := &fakeDeadlineResponseWriter{ResponseRecorder: httptest.NewRecorder(), err: deadlineErr}

	s.Handler().ServeHTTP(writer, req)

	if writer.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", writer.Code)
	}
	if strings.Contains(writer.Body.String(), ": connected") {
		t.Fatalf("body = %q, want no connected stream frame", writer.Body.String())
	}
	if !unsubscribed {
		t.Fatal("write-deadline failure did not unsubscribe")
	}
}

func newNotificationStreamServer(t *testing.T, keepalive time.Duration) (*Server, token.Maker, *notification.Hub) {
	t.Helper()
	maker, err := token.NewJWTMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	hub := notification.NewHub()
	s, err := NewServer(config.Config{JWTSecret: testSecret}, fakeStore{}, maker, nil, hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.notificationKeepalive = keepalive
	return s, maker, hub
}

func openNotificationStream(
	t *testing.T,
	testServer *httptest.Server,
	maker token.Maker,
	username string,
	tokenTTL time.Duration,
) (*http.Response, context.CancelFunc) {
	t.Helper()
	accessToken, _, err := maker.CreateToken(username, roleDepositor, token.Access, tokenTTL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, testServer.URL+"/api/v1/notifications/stream", nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := testServer.Client().Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		cancel()
		defer func() { _ = resp.Body.Close() }()
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (%s)", resp.StatusCode, raw)
	}
	return resp, cancel
}

func assertNotificationStreamHeaders(t *testing.T, resp *http.Response) {
	t.Helper()
	for name, want := range map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-store",
		"Connection":        "keep-alive",
		"X-Accel-Buffering": "no",
	} {
		if got := resp.Header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func readSSEFrame(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	var frame strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("reading SSE frame: %v", err)
		}
		frame.WriteString(line)
		if line == "\n" {
			return frame.String()
		}
	}
}

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
