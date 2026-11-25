package event

import (
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
)

const (
	PageView      = "page_view"
	ProductView   = "product_view"
	AddToCart     = "add_to_cart"
	CheckoutStart = "checkout_start"
	Purchase      = "purchase"
)

const (
	MaxFuture = 5 * time.Minute
	MaxAge    = 24 * time.Hour
	MaxBatch  = 500
)

var needsProduct = map[string]bool{
	ProductView:   true,
	AddToCart:     true,
	CheckoutStart: true,
	Purchase:      true,
}

var knownTypes = map[string]bool{
	PageView:      true,
	ProductView:   true,
	AddToCart:     true,
	CheckoutStart: true,
	Purchase:      true,
}

var knownDevices = map[string]bool{"": true, "desktop": true, "mobile": true, "tablet": true}

var countryCode = regexp.MustCompile(`^[A-Z]{2}$`)

type Input struct {
	EventID    string `json:"event_id"`
	Type       string `json:"type"`
	OccurredAt string `json:"occurred_at"`
	SessionID  string `json:"session_id"`
	UserID     string `json:"user_id"`
	ProductID  string `json:"product_id"`
	Category   string `json:"category"`
	Price      uint64 `json:"price"`
	Country    string `json:"country"`
	Device     string `json:"device"`
	URL        string `json:"url"`
}

type Event struct {
	EventID    uuid.UUID
	Type       string
	OccurredAt time.Time
	ReceivedAt time.Time
	SessionID  string
	UserID     string
	ProductID  string
	Category   string
	Price      uint64
	Country    string
	Device     string
	URL        string
}

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e FieldError) Error() string {
	return e.Field + ": " + e.Message
}

func Parse(in Input, now time.Time) (Event, error) {
	id, err := uuid.Parse(in.EventID)
	if err != nil {
		return Event{}, FieldError{"event_id", "must be a UUID"}
	}
	if !knownTypes[in.Type] {
		return Event{}, FieldError{"type", fmt.Sprintf("unknown event type %q", in.Type)}
	}
	occurred, err := time.Parse(time.RFC3339Nano, in.OccurredAt)
	if err != nil {
		return Event{}, FieldError{"occurred_at", "must be an RFC 3339 time such as 2026-11-25T18:00:00Z"}
	}
	if occurred.After(now.Add(MaxFuture)) {
		return Event{}, FieldError{"occurred_at", "is more than 5 minutes in the future"}
	}
	if occurred.Before(now.Add(-MaxAge)) {
		return Event{}, FieldError{"occurred_at", "is more than 24 hours old"}
	}
	if err := checkLength("session_id", in.SessionID, 1, 128); err != nil {
		return Event{}, err
	}
	if err := checkLength("user_id", in.UserID, 0, 128); err != nil {
		return Event{}, err
	}
	minProduct := 0
	if needsProduct[in.Type] {
		minProduct = 1
	}
	if err := checkLength("product_id", in.ProductID, minProduct, 64); err != nil {
		return Event{}, err
	}
	if err := checkLength("category", in.Category, 0, 64); err != nil {
		return Event{}, err
	}
	if in.Type == Purchase && in.Price == 0 {
		return Event{}, FieldError{"price", "is required for a purchase (in kuruş)"}
	}
	if in.Country != "" && !countryCode.MatchString(in.Country) {
		return Event{}, FieldError{"country", "must be a two-letter code such as TR"}
	}
	if !knownDevices[in.Device] {
		return Event{}, FieldError{"device", "must be desktop, mobile or tablet"}
	}
	if err := checkLength("url", in.URL, 0, 2048); err != nil {
		return Event{}, err
	}

	return Event{
		EventID:    id,
		Type:       in.Type,
		OccurredAt: occurred.UTC(),
		ReceivedAt: now.UTC(),
		SessionID:  in.SessionID,
		UserID:     in.UserID,
		ProductID:  in.ProductID,
		Category:   in.Category,
		Price:      in.Price,
		Country:    in.Country,
		Device:     in.Device,
		URL:        in.URL,
	}, nil
}

func checkLength(field, value string, minLen, maxLen int) error {
	if len(value) < minLen {
		return FieldError{field, "is required"}
	}
	if len(value) > maxLen {
		return FieldError{field, fmt.Sprintf("is longer than %d characters", maxLen)}
	}
	return nil
}
