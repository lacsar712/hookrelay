package replay

import (
	"fmt"
	"time"

	"github.com/lacsar712/hookrelay/internal/dlq"
	"github.com/lacsar712/hookrelay/internal/idgen"
	"github.com/lacsar712/hookrelay/internal/job"
	"github.com/lacsar712/hookrelay/internal/journal"
)

func FromJournal(e journal.Entry, now time.Time) (job.Job, error) {
	if len(e.Body) == 0 {
		return job.Job{}, fmt.Errorf("journal entry %s has no stored body", e.DeliveryID)
	}
	return job.Job{
		EventID:       e.EventID,
		DeliveryID:    idgen.New("dlv", now),
		DestinationID: e.DestinationID,
		Type:          e.Type,
		Body:          append([]byte(nil), e.Body...),
		Attempt:       0,
		NotBefore:     now,
		CreatedAt:     now,
		ReplayOf:      e.DeliveryID,
	}, nil
}

func FromDLQ(it dlq.Item, now time.Time) (job.Job, error) {
	if len(it.Body) == 0 {
		return job.Job{}, fmt.Errorf("dlq item %s has no stored body", it.DeliveryID)
	}
	return job.Job{
		EventID:       it.EventID,
		DeliveryID:    idgen.New("dlv", now),
		DestinationID: it.DestinationID,
		Type:          it.Type,
		Body:          append([]byte(nil), it.Body...),
		Attempt:       0,
		NotBefore:     now,
		CreatedAt:     now,
		ReplayOf:      it.DeliveryID,
	}, nil
}
