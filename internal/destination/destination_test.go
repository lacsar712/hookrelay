package destination_test

import (
	"testing"
	"time"

	"github.com/lacsar712/hookrelay/internal/clock"
	"github.com/lacsar712/hookrelay/internal/destination"
)

func TestMatchesSegmentBoundary(t *testing.T) {
	cases := []struct {
		prefix    string
		eventType string
		want      bool
	}{
		{"order", "order.paid", true},
		{"order.", "order.paid", true},
		{"order.paid", "order.paid", true},
		{"", "anything.else", true},
		{"or", "order.paid", false},
		{"ord", "order.paid", false},
		{"order.p", "order.paid", false},
		{"charge", "order.paid", false},
		{"order", "orders.paid", false},
	}
	for _, tc := range cases {
		d := destination.Destination{
			Enabled:      true,
			TypePrefixes: []string{tc.prefix},
		}
		got := d.Matches(tc.eventType)
		if got != tc.want {
			t.Fatalf("prefix %q vs type %q: Matches=%v want %v", tc.prefix, tc.eventType, got, tc.want)
		}
	}
}

func TestMatchesDisabledNeverFires(t *testing.T) {
	d := destination.Destination{
		Enabled:      false,
		TypePrefixes: []string{""},
	}
	if d.Matches("order.paid") {
		t.Fatal("disabled destination must not match")
	}
}

func TestRegistryMatchingSkipsDisabled(t *testing.T) {
	clk := clock.NewFrozen(time.Unix(0, 0))
	reg := destination.NewRegistry(clk)
	off := false
	d, err := reg.Create(destination.CreateInput{
		Name:         "off",
		URL:          "http://127.0.0.1:8080/api/v1/loopback",
		Secret:       "abcdefgh",
		TypePrefixes: []string{"order"},
		Enabled:      &off,
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Enabled {
		t.Fatal("expected disabled")
	}
	got := reg.Matching("order.paid")
	if len(got) != 0 {
		t.Fatalf("disabled dest leaked into matching: %+v", got)
	}
}
