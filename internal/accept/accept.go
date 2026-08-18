package accept

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/lacsar712/hookrelay/internal/clock"
	"github.com/lacsar712/hookrelay/internal/destination"
	"github.com/lacsar712/hookrelay/internal/event"
	"github.com/lacsar712/hookrelay/internal/hashutil"
	"github.com/lacsar712/hookrelay/internal/headers"
	"github.com/lacsar712/hookrelay/internal/idgen"
	"github.com/lacsar712/hookrelay/internal/idempotency"
	"github.com/lacsar712/hookrelay/internal/ingest"
	"github.com/lacsar712/hookrelay/internal/job"
	"github.com/lacsar712/hookrelay/internal/nonce"
	"github.com/lacsar712/hookrelay/internal/queue"
	"github.com/lacsar712/hookrelay/internal/route"
	"github.com/lacsar712/hookrelay/internal/sign"
)

type Pipeline struct {
	Clk    clock.Clock
	Window time.Duration
	Keys   *ingest.Keys
	Nonces *nonce.Book
	Idem   *idempotency.Store
	Dests  *destination.Registry
	Broker *queue.Broker
}

type Result struct {
	EventID     string   `json:"event_id"`
	Replay      bool     `json:"replay"`
	Matched     int      `json:"matched"`
	DeliveryIDs []string `json:"delivery_ids"`
}

func (p *Pipeline) Handle(h http.Header, body []byte) (Result, int, error) {
	in, err := headers.ParseInbound(h)
	if err != nil {
		return Result{}, http.StatusBadRequest, err
	}
	if err := hashutil.ValidIdempotencyKey(in.IdemKey); err != nil {
		return Result{}, http.StatusBadRequest, err
	}
	secrets := p.Keys.Secrets(in.SourceKey)
	if len(secrets) == 0 {
		return Result{}, http.StatusUnauthorized, fmt.Errorf("unknown source key %q", in.SourceKey)
	}
	if err := sign.Verify(p.Clk, p.Window, secrets, sign.Headers{
		Timestamp: in.Timestamp,
		Nonce:     in.Nonce,
		Signature: in.Signature,
	}, body); err != nil {
		if err == sign.ErrSkew {
			return Result{}, http.StatusBadRequest, err
		}
		return Result{}, http.StatusUnauthorized, err
	}
	env, err := event.Parse(body)
	if err != nil {
		var syn *json.SyntaxError
		if errors.As(err, &syn) {
			return Result{}, http.StatusBadRequest, err
		}
		return Result{}, http.StatusUnprocessableEntity, err
	}
	if err := p.Nonces.CheckAndRemember(in.Nonce); err != nil {
		return Result{}, http.StatusConflict, err
	}
	now := p.Clk.Now()
	eventID := idgen.New("evt", now)
	bodyHash := hashutil.SHA256Hex(body)
	existing, replay, err := p.Idem.Remember(in.IdemKey, bodyHash, eventID)
	if err != nil {
		if errors.Is(err, idempotency.ErrConflict) {
			return Result{}, http.StatusConflict, err
		}
		return Result{}, http.StatusBadRequest, err
	}
	if replay {
		return Result{EventID: existing, Replay: true}, http.StatusOK, nil
	}
	matched := p.Dests.Matching(env.Type)
	plan := route.Fanout(eventID, env.Type, body, matched, now)
	ids := make([]string, 0, len(plan.Items))
	for _, item := range plan.Items {
		d, ok := p.Dests.Get(item.DestinationID)
		if !ok {
			continue
		}
		p.Broker.Ensure(d.ID, d.Ordered, d.MaxInFlight)
		p.Broker.Enqueue(job.Job{
			EventID:       eventID,
			DeliveryID:    item.DeliveryID,
			DestinationID: item.DestinationID,
			Type:          env.Type,
			Body:          append([]byte(nil), body...),
			Attempt:       0,
			NotBefore:     now,
			CreatedAt:     now,
		}, d.Ordered, d.MaxInFlight)
		ids = append(ids, item.DeliveryID)
	}
	return Result{
		EventID:     eventID,
		Matched:     len(ids),
		DeliveryIDs: ids,
	}, http.StatusAccepted, nil
}
