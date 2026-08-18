package event_test

import (
	"testing"

	"github.com/lacsar712/hookrelay/internal/event"
)

func TestParseAndPrefix(t *testing.T) {
	env, err := event.Parse([]byte(`{"type":"order.paid","payload":{"id":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	if env.Type != "order.paid" {
		t.Fatalf("type %s", env.Type)
	}
	if !event.MatchPrefix("order.paid", "order") {
		t.Fatal("order should match order.paid")
	}
	if event.MatchPrefix("order.paid", "charge") {
		t.Fatal("charge should not match")
	}
	if _, err := event.Parse([]byte(`{"type":"Order.Paid","payload":{}}`)); err == nil {
		t.Fatal("uppercase type should fail")
	}
}

func TestMatchPrefixSegmentBoundary(t *testing.T) {
	cases := []struct {
		eventType string
		prefix    string
		want      bool
	}{
		{"order.paid", "order", true},
		{"order.paid", "order.", true},
		{"order.paid", "order.paid", true},
		{"order.paid", "", true},
		{"order.paid", "or", false},
		{"order.paid", "ord", false},
		{"order.paid", "order.p", false},
		{"order.paid", "charge", false},
		{"orders.paid", "order", false},
	}
	for _, tc := range cases {
		got := event.MatchPrefix(tc.eventType, tc.prefix)
		if got != tc.want {
			t.Fatalf("MatchPrefix(%q, %q)=%v want %v", tc.eventType, tc.prefix, got, tc.want)
		}
	}
}
