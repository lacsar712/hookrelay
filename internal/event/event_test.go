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
