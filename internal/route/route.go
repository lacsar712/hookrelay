package route

import (
	"time"

	"github.com/lacsar712/hookrelay/internal/destination"
	"github.com/lacsar712/hookrelay/internal/idgen"
)

type PlanItem struct {
	DestinationID string
	DeliveryID    string
	URL           string
	Ordered       bool
}

type Plan struct {
	EventID    string
	Type       string
	Items      []PlanItem
	DroppedOff int
}

func Fanout(eventID, eventType string, body []byte, dests []destination.Destination, now time.Time) Plan {
	_ = body
	p := Plan{EventID: eventID, Type: eventType, Items: make([]PlanItem, 0, len(dests))}
	for _, d := range dests {
		if !d.Matches(eventType) {
			p.DroppedOff++
			continue
		}
		p.Items = append(p.Items, PlanItem{
			DestinationID: d.ID,
			DeliveryID:    idgen.New("dlv", now),
			URL:           d.URL,
			Ordered:       d.Ordered,
		})
	}
	return p
}
