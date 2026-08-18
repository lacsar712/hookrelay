package accept_test

import (
	"net/http"
	"strconv"
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

func newPipeline(t *testing.T, clk *clock.Frozen) *accept.Pipeline {
	t.Helper()
	dests := destination.NewRegistry(clk)
	enabled := true
	if _, err := dests.Create(destination.CreateInput{
		Name:         "loop",
		URL:          "http://127.0.0.1:8080/api/v1/loopback",
		Secret:       "abcdefgh",
		TypePrefixes: []string{"order"},
		Enabled:      &enabled,
	}); err != nil {
		t.Fatal(err)
	}
	return &accept.Pipeline{
		Clk:    clk,
		Window: 5 * time.Minute,
		Keys:   ingest.New("ingest", "supersecret"),
		Nonces: nonce.New(clk, 5*time.Minute),
		Idem:   idempotency.New(clk, time.Hour),
		Dests:  dests,
		Broker: queue.NewBroker(clk),
	}
}

func signedHeaders(t *testing.T, body []byte, nonce, idem string, ts int64) http.Header {
	t.Helper()
	sig, err := sign.Sign("supersecret", ts, nonce, body)
	if err != nil {
		t.Fatal(err)
	}
	h := make(http.Header)
	h.Set(headers.Timestamp, strconv.FormatInt(ts, 10))
	h.Set(headers.Nonce, nonce)
	h.Set(headers.Signature, sig)
	h.Set(headers.Idempotency, idem)
	h.Set(headers.SourceKey, "ingest")
	return h
}

func TestPipelineSkewIsBadRequest(t *testing.T) {
	clk := clock.NewFrozen(time.Unix(1_700_000_000, 0))
	p := newPipeline(t, clk)
	body := []byte(`{"type":"order.paid","payload":{"id":1}}`)
	ts := clk.Now().Add(-20 * time.Minute).Unix()
	_, code, err := p.Handle(signedHeaders(t, body, "skewnonce16chars", "idem-skew-01", ts), body)
	if err == nil {
		t.Fatal("expected skew error")
	}
	if code != http.StatusBadRequest {
		t.Fatalf("code %d want 400", code)
	}
}

func TestPipelineIdempotencyConflict(t *testing.T) {
	clk := clock.NewFrozen(time.Unix(1_700_000_000, 0))
	p := newPipeline(t, clk)
	body1 := []byte(`{"type":"order.paid","payload":{"id":1}}`)
	body2 := []byte(`{"type":"order.paid","payload":{"id":2}}`)
	ts := clk.Now().Unix()
	_, code1, err := p.Handle(signedHeaders(t, body1, "idemnonce16charA", "same-idem-key", ts), body1)
	if err != nil || code1 != http.StatusAccepted {
		t.Fatalf("first: code=%d err=%v", code1, err)
	}
	_, code2, err := p.Handle(signedHeaders(t, body2, "idemnonce16charB", "same-idem-key", ts), body2)
	if err == nil {
		t.Fatal("expected conflict")
	}
	if code2 != http.StatusConflict {
		t.Fatalf("code %d want 409", code2)
	}
}
