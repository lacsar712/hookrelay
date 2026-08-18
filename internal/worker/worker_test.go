package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lacsar712/hookrelay/internal/backoff"
	"github.com/lacsar712/hookrelay/internal/clock"
	"github.com/lacsar712/hookrelay/internal/deliver"
	"github.com/lacsar712/hookrelay/internal/destination"
	"github.com/lacsar712/hookrelay/internal/dlq"
	"github.com/lacsar712/hookrelay/internal/job"
	"github.com/lacsar712/hookrelay/internal/journal"
	"github.com/lacsar712/hookrelay/internal/queue"
	"github.com/lacsar712/hookrelay/internal/runtime"
)

func newTestEngine(t *testing.T, url string, clk clock.Clock) (*Engine, *destination.Destination) {
	t.Helper()
	dests := destination.NewRegistry(clk)
	enabled := true
	d, err := dests.Create(destination.CreateInput{
		Name:         "t",
		URL:          url,
		Secret:       "abcdefgh",
		TypePrefixes: []string{""},
		Rate:         100,
		Burst:        100,
		MaxInFlight:  4,
		Enabled:      &enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	broker := queue.NewBroker(clk)
	broker.Ensure(d.ID, d.Ordered, d.MaxInFlight)
	e := New(clk, broker, dests, runtime.NewGates(clk), deliver.New(2*time.Second), journal.New(50), dlq.New(50), backoff.Policy{
		Base:        time.Millisecond,
		Cap:         time.Millisecond,
		MaxAttempts: 8,
	})
	return e, &d
}

func TestHandleRetriesStatus429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	clk := clock.NewFrozen(time.Unix(1_700_000_000, 0))
	e, dest := newTestEngine(t, srv.URL, clk)
	j := job.Job{
		EventID:       "evt_test",
		DeliveryID:    "dlv_test429",
		DestinationID: dest.ID,
		Type:          "order.paid",
		Body:          []byte(`{"type":"order.paid","payload":{}}`),
		Attempt:       0,
		NotBefore:     clk.Now(),
		CreatedAt:     clk.Now(),
	}
	e.handle(context.Background(), j)
	if e.dead.Len() != 0 {
		t.Fatalf("429 should not go to dlq, got %d", e.dead.Len())
	}
	if e.broker.Depth() != 1 {
		t.Fatalf("429 should requeue, depth=%d", e.broker.Depth())
	}
}

func TestHandleTreats202AsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	clk := clock.NewFrozen(time.Unix(1_700_000_000, 0))
	e, dest := newTestEngine(t, srv.URL, clk)
	j := job.Job{
		EventID:       "evt_test",
		DeliveryID:    "dlv_test202",
		DestinationID: dest.ID,
		Type:          "order.paid",
		Body:          []byte(`{"type":"order.paid","payload":{}}`),
		Attempt:       0,
		NotBefore:     clk.Now(),
		CreatedAt:     clk.Now(),
	}
	e.handle(context.Background(), j)
	if e.dead.Len() != 0 {
		t.Fatalf("202 should not go to dlq, got %d", e.dead.Len())
	}
	if e.broker.Depth() != 0 {
		t.Fatalf("202 should not requeue, depth=%d", e.broker.Depth())
	}
	entries := e.log.List("", 10)
	if len(entries) == 0 || entries[0].Kind != "success" {
		t.Fatalf("want success journal, got %+v", entries)
	}
}
