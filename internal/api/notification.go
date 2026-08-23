package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"uuid"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

type notificationResponse struct {
	ID         uuid.UUID  `json:"id"`
	AccountID  uuid.UUID  `json:"account_id"`
	TransferID uuid.UUID  `json:"transfer_id"`
	Direction  string     `json:"direction"`
	Amount     int64      `json:"amount"`
	Currency   string     `json:"currency"`
	Balance    int64      `json:"balance"`
	ReadAt     *time.Time `json:"read_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

type listNotificationsResponse struct {
	Notifications []notificationResponse `json:"notifications"`
	UnreadCount   int64                  `json:"unread_count"`
	NextCursor    *string                `json:"next_cursor"`
}

type notificationReadResponse struct {
	UnreadCount int64 `json:"unread_count"`
}

type notificationCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

var errInvalidNotificationCursor = errors.New("invalid notification cursor")

func (s *Server) listNotifications(c *echo.Context) error {
	size, err := echo.QueryParamOr[int32](c, "size", 20)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid size")
	}
	if size < 1 {
		size = 20
	}
	size = min(size, 100)

	arg := store.ListNotificationsPageParams{Limit: size}
	if values, present := c.Request().URL.Query()["cursor"]; present {
		if len(values) != 1 {
			return invalidNotificationCursor()
		}
		cursor, decodeErr := decodeNotificationCursor(values[0])
		if decodeErr != nil {
			return invalidNotificationCursor()
		}
		arg.HasCursor = true
		arg.CursorCreatedAt = cursor.CreatedAt
		arg.CursorID = cursor.ID
	}

	payload, err := authPayload(c)
	if err != nil {
		return err
	}
	arg.Owner = payload.Username

	page, err := s.store.ListNotificationsPage(c.Request().Context(), arg)
	if err != nil {
		return err
	}

	notifications := make([]notificationResponse, len(page.Notifications))
	for i, notification := range page.Notifications {
		notifications[i] = newNotificationResponse(notification)
	}

	var nextCursor *string
	if page.HasMore && len(page.Notifications) > 0 {
		last := page.Notifications[len(page.Notifications)-1]
		encoded, encodeErr := encodeNotificationCursor(notificationCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if encodeErr != nil {
			return encodeErr
		}
		nextCursor = &encoded
	}

	return c.JSON(http.StatusOK, listNotificationsResponse{
		Notifications: notifications,
		UnreadCount:   page.UnreadCount,
		NextCursor:    nextCursor,
	})
}

func (s *Server) markNotificationRead(c *echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid notification id")
	}
	payload, err := authPayload(c)
	if err != nil {
		return err
	}
	unreadCount, err := s.store.MarkNotificationReadTx(c.Request().Context(), payload.Username, id)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, notificationReadResponse{UnreadCount: unreadCount})
}

func (s *Server) markAllNotificationsRead(c *echo.Context) error {
	payload, err := authPayload(c)
	if err != nil {
		return err
	}
	unreadCount, err := s.store.MarkAllNotificationsReadTx(c.Request().Context(), payload.Username)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, notificationReadResponse{UnreadCount: unreadCount})
}

func newNotificationResponse(notification sqlcdb.Notification) notificationResponse {
	var readAt *time.Time
	if notification.ReadAt.Valid {
		value := notification.ReadAt.Time
		readAt = &value
	}
	return notificationResponse{
		ID:         notification.ID,
		AccountID:  notification.AccountID,
		TransferID: notification.TransferID,
		Direction:  notification.Direction,
		Amount:     notification.Amount,
		Currency:   notification.Currency,
		Balance:    notification.Balance,
		ReadAt:     readAt,
		CreatedAt:  notification.CreatedAt,
	}
}

func decodeNotificationCursor(value string) (notificationCursor, error) {
	var cursor notificationCursor
	raw, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil {
		return cursor, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return cursor, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return notificationCursor{}, errInvalidNotificationCursor
	}
	if cursor.CreatedAt.IsZero() || cursor.ID == uuid.Nil() {
		return notificationCursor{}, errInvalidNotificationCursor
	}
	return cursor, nil
}

func encodeNotificationCursor(cursor notificationCursor) (string, error) {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func invalidNotificationCursor() error {
	return echo.NewHTTPError(http.StatusBadRequest, "invalid notification cursor")
}
