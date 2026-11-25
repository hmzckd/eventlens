package event

import (
	"errors"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 25, 18, 0, 0, 0, time.UTC)

func validInput() Input {
	return Input{
		EventID:    "0193b4c2-7d1e-7a3b-9c4d-5e6f7a8b9c0d",
		Type:       ProductView,
		OccurredAt: "2026-11-25T17:59:30Z",
		SessionID:  "s-1",
		ProductID:  "p-42",
		Category:   "shoes",
		Country:    "TR",
		Device:     "mobile",
		URL:        "/products/p-42",
	}
}

func TestParseAcceptsAValidEvent(t *testing.T) {
	ev, err := Parse(validInput(), now)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if ev.EventID.String() != "0193b4c2-7d1e-7a3b-9c4d-5e6f7a8b9c0d" {
		t.Errorf("EventID = %s", ev.EventID)
	}
	if !ev.OccurredAt.Equal(now.Add(-30 * time.Second)) {
		t.Errorf("OccurredAt = %s", ev.OccurredAt)
	}
	if !ev.ReceivedAt.Equal(now) {
		t.Errorf("ReceivedAt = %s, want the server time", ev.ReceivedAt)
	}
}

func TestParseConvertsOffsetsToUTC(t *testing.T) {
	in := validInput()
	in.OccurredAt = "2026-11-25T20:59:30+03:00"
	ev, err := Parse(in, now)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if ev.OccurredAt.Location() != time.UTC || !ev.OccurredAt.Equal(now.Add(-30*time.Second)) {
		t.Fatalf("OccurredAt = %s", ev.OccurredAt)
	}
}

func TestParseRejectsBadFields(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Input)
		field  string
	}{
		{"event id not a uuid", func(in *Input) { in.EventID = "42" }, "event_id"},
		{"unknown type", func(in *Input) { in.Type = "click" }, "type"},
		{"time not rfc 3339", func(in *Input) { in.OccurredAt = "25.11.2026 18:00" }, "occurred_at"},
		{"time too far in the future", func(in *Input) { in.OccurredAt = "2026-11-25T18:06:00Z" }, "occurred_at"},
		{"time older than a day", func(in *Input) { in.OccurredAt = "2026-11-24T17:59:59Z" }, "occurred_at"},
		{"missing session", func(in *Input) { in.SessionID = "" }, "session_id"},
		{"product view without product", func(in *Input) { in.ProductID = "" }, "product_id"},
		{"purchase without price", func(in *Input) { in.Type = Purchase; in.Price = 0 }, "price"},
		{"lowercase country", func(in *Input) { in.Country = "tr" }, "country"},
		{"unknown device", func(in *Input) { in.Device = "fridge" }, "device"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			tc.change(&in)
			_, err := Parse(in, now)
			var fe FieldError
			if !errors.As(err, &fe) {
				t.Fatalf("err = %v, want a FieldError", err)
			}
			if fe.Field != tc.field {
				t.Fatalf("field = %q, want %q", fe.Field, tc.field)
			}
		})
	}
}

func TestParseAllowsPageViewWithoutProduct(t *testing.T) {
	in := validInput()
	in.Type = PageView
	in.ProductID = ""
	if _, err := Parse(in, now); err != nil {
		t.Fatalf("Parse: %v", err)
	}
}

func TestParseAcceptsTheEdgesOfTheTimeWindow(t *testing.T) {
	for _, at := range []string{"2026-11-25T18:05:00Z", "2026-11-24T18:00:00Z"} {
		in := validInput()
		in.OccurredAt = at
		if _, err := Parse(in, now); err != nil {
			t.Errorf("%s: %v", at, err)
		}
	}
}
