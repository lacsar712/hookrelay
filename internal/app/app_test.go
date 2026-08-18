package app_test

import (
	"testing"
	"time"

	"github.com/lacsar712/hookrelay/internal/app"
	"github.com/lacsar712/hookrelay/internal/config"
	"github.com/lacsar712/hookrelay/internal/journal"
)

func TestReplayEmptyJournalBodyFails(t *testing.T) {
	a, err := app.New(config.Config{
		Addr:         ":0",
		DataDir:      t.TempDir(),
		IngestSecret: "dev-ingest-secret",
		Window:       5 * time.Minute,
		IdemTTL:      time.Hour,
		Workers:      1,
		PublicBase:   "http://127.0.0.1:8080",
		LoopbackPath: "/api/v1/loopback",
	})
	if err != nil {
		t.Fatal(err)
	}
	dests := a.Dests.List()
	if len(dests) == 0 {
		t.Fatal("expected seeded destination")
	}
	a.Log.Append(journal.Entry{
		EventID:       "evt-empty",
		DeliveryID:    "dlv-empty-body",
		DestinationID: dests[0].ID,
		Type:          "order.paid",
	})
	if _, err := a.Replay("dlv-empty-body"); err == nil {
		t.Fatal("replay of empty journal body must fail")
	}
	if a.Broker.Depth() != 0 {
		t.Fatalf("failed replay must not enqueue, depth=%d", a.Broker.Depth())
	}
}
