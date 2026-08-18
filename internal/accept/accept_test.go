package accept_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/lacsar712/hookrelay/internal/accept"
	"github.com/lacsar712/hookrelay/internal/clock"
	"github.com/lacsar712/hookrelay/internal/destination"
	"github.com/lacsar712/hookrelay/internal/hashutil"
	"github.com/lacsar712/hookrelay/internal/headers"
	"github.com/lacsar712/hookrelay/internal/idempotency"
	"github.com/lacsar712/hookrelay/internal/ingest"
	"github.com/lacsar712/hookrelay/internal/nonce"
	"github.com/lacsar712/hookrelay/internal/queue"
	"github.com/lacsar712/hookrelay/internal/sign"
)

func TestPipelineAcceptsSignedEvent(t *testing.T) {
	clk := clock.NewFrozen(time.Unix(1_700_000_000, 0))
	dests := destination.NewRegistry(clk)
	enabled := true
	_, err := dests.Create(destination.CreateInput{
		Name:         "loop",
		URL:          "http://127.0.0.1:8080/api/v1/loopback",
		Secret:       "abcdefgh",
		TypePrefixes: []string{"order"},
		Enabled:      &enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := &accept.Pipeline{
		Clk:    clk,
		Window: 5 * time.Minute,
		Keys:   ingest.New("ingest", "supersecret"),
		Nonces: nonce.New(clk, 5*time.Minute),
		Idem:   idempotency.New(clk, time.Hour),
		Dests:  dests,
		Broker: queue.NewBroker(clk),
	}
	body := []byte(`{"type":"order.paid","payload":{"id":1}}`)
	n := "abcdefghijklmnop"
	ts := clk.Now().Unix()
	sig, err := sign.Sign("supersecret", ts, n, body)
	if err != nil {
		t.Fatal(err)
	}
	h := make(http.Header)
	h.Set(headers.Timestamp, "1700000000")
	h.Set(headers.Nonce, n)
	h.Set(headers.Signature, sig)
	h.Set(headers.Idempotency, "idemkey1")
	h.Set(headers.SourceKey, "ingest")
	res, code, err := p.Handle(h, body)
	if err != nil {
		t.Fatal(err)
	}
	if code != http.StatusAccepted {
		t.Fatalf("code %d", code)
	}
	if res.Matched != 1 {
		t.Fatalf("matched %d hash %s", res.Matched, hashutil.SHA256Hex(body))
	}
}

func TestPipelineRejectsPartialTypePrefix(t *testing.T) {
	clk := clock.NewFrozen(time.Unix(1_700_000_000, 0))
	dests := destination.NewRegistry(clk)
	enabled := true
	_, err := dests.Create(destination.CreateInput{
		Name:         "too-short",
		URL:          "http://127.0.0.1:8080/api/v1/loopback",
		Secret:       "abcdefgh",
		TypePrefixes: []string{"or"},
		Enabled:      &enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := &accept.Pipeline{
		Clk:    clk,
		Window: 5 * time.Minute,
		Keys:   ingest.New("ingest", "supersecret"),
		Nonces: nonce.New(clk, 5*time.Minute),
		Idem:   idempotency.New(clk, time.Hour),
		Dests:  dests,
		Broker: queue.NewBroker(clk),
	}
	body := []byte(`{"type":"order.paid","payload":{"id":1}}`)
	n := "qrstuvwxyzabcdef"
	ts := clk.Now().Unix()
	sig, err := sign.Sign("supersecret", ts, n, body)
	if err != nil {
		t.Fatal(err)
	}
	h := make(http.Header)
	h.Set(headers.Timestamp, "1700000000")
	h.Set(headers.Nonce, n)
	h.Set(headers.Signature, sig)
	h.Set(headers.Idempotency, "idemkey-partial")
	h.Set(headers.SourceKey, "ingest")
	res, code, err := p.Handle(h, body)
	if err != nil {
		t.Fatal(err)
	}
	if code != http.StatusAccepted {
		t.Fatalf("code %d", code)
	}
	if res.Matched != 0 {
		t.Fatalf("prefix %q must not match %q, matched=%d ids=%v", "or", "order.paid", res.Matched, res.DeliveryIDs)
	}
}
