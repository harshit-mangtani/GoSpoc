package redisstream

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/harshit-mangtani/GoSpoc/internal/queue"
	"github.com/redis/go-redis/v9"
)

const fieldSubmissionID = "submission_id"

// blockDuration is how long Consume waits for a new message before giving up
// and returning queue.ErrNoMessage. Keeping it bounded lets the worker loop
// check for shutdown between reads.
const blockDuration = 5 * time.Second

type Queue struct {
	client *redis.Client
	stream string
	group  string
}

// New creates the stream and consumer group if they don't already exist.
// Using MkStream means we don't need the stream to pre-exist; "$" starts the
// group reading only messages added after creation.
func New(ctx context.Context, client *redis.Client, stream, group string) (*Queue, error) {
	q := &Queue{
		client: client,
		stream: stream,
		group:  group,
	}

	err := client.XGroupCreateMkStream(ctx, stream, group, "$").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return nil, err
	}

	return q, nil
}

// Enqueue appends a job to the stream (XADD).
func (q *Queue) Enqueue(ctx context.Context, job queue.Job) error {
	return q.client.XAdd(ctx, &redis.XAddArgs{
		Stream: q.stream,
		Values: map[string]any{
			fieldSubmissionID: strconv.FormatInt(job.SubmissionID, 10),
		},
	}).Err()
}

// Consume reads the next un-delivered message for this consumer group
// (XREADGROUP with ">"). It blocks up to blockDuration; if nothing arrives it
// returns queue.ErrNoMessage. The message stays in the group's pending list
// until Ack'd.
func (q *Queue) Consume(ctx context.Context, consumer string) (queue.Message, error) {
	res, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    q.group,
		Consumer: consumer,
		Streams:  []string{q.stream, ">"},
		Count:    1,
		Block:    blockDuration,
	}).Result()

	if err == redis.Nil {
		return queue.Message{}, queue.ErrNoMessage
	}
	if err != nil {
		return queue.Message{}, err
	}
	if len(res) == 0 || len(res[0].Messages) == 0 {
		return queue.Message{}, queue.ErrNoMessage
	}

	raw := res[0].Messages[0]

	submissionID, err := parseSubmissionID(raw.Values)
	if err != nil {
		// Poison message: ack it so it doesn't get redelivered forever, and
		// surface the error to the caller.
		_ = q.client.XAck(ctx, q.stream, q.group, raw.ID).Err()
		return queue.Message{}, err
	}

	return queue.Message{
		ID:  raw.ID,
		Job: queue.Job{SubmissionID: submissionID},
	}, nil
}

// Ack confirms a message is fully processed and removes it from the pending
// list (XACK).
func (q *Queue) Ack(ctx context.Context, msg queue.Message) error {
	return q.client.XAck(ctx, q.stream, q.group, msg.ID).Err()
}

// Nack signals processing failed. We deliberately do NOT ack, so the message
// stays in the pending list and can be reclaimed later (XAUTOCLAIM, added with
// the worker in a later phase). The DB-backed sweeper is the ultimate backstop
// for anything that gets stuck here.
func (q *Queue) Nack(ctx context.Context, msg queue.Message) error {
	return nil
}

func parseSubmissionID(values map[string]any) (int64, error) {
	raw, _ := values[fieldSubmissionID].(string)
	return strconv.ParseInt(raw, 10, 64)
}

// compile-time check that *Queue satisfies the interface.
var _ queue.Queue = (*Queue)(nil)
