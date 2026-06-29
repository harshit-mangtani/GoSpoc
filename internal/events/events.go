package events

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/redis/go-redis/v9"
)

type Event struct {
	SubmissionID int64   `json:"submission_id"`
	Status       string  `json:"status"`
	Verdict      *string `json:"verdict,omitempty"`
	RuntimeMS    *int    `json:"runtime_ms,omitempty"`
	MemoryKB     *int    `json:"memory_kb,omitempty"`
}

func channel(submissionID int64) string {
	return "submission-events:" + strconv.FormatInt(submissionID, 10)
}

type Publisher struct{ client *redis.Client }

func NewPublisher(client *redis.Client) *Publisher { return &Publisher{client: client} }

func (p *Publisher) Publish(ctx context.Context, e Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return p.client.Publish(ctx, channel(e.SubmissionID), data).Err()
}

type Subscriber struct{ client *redis.Client }

func NewSubscriber(client *redis.Client) *Subscriber { return &Subscriber{client: client} }

// Subscribe returns a channel of events for one submission plus a cancel func
// that releases the subscription (and closes the channel).
func (s *Subscriber) Subscribe(ctx context.Context, submissionID int64) (<-chan Event, func()) {
	pubsub := s.client.Subscribe(ctx, channel(submissionID))
	out := make(chan Event)

	go func() {
		defer close(out)
		for msg := range pubsub.Channel() {
			var e Event
			if err := json.Unmarshal([]byte(msg.Payload), &e); err != nil {
				continue
			}
			select {
			case out <- e:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, func() { _ = pubsub.Close() }
}
