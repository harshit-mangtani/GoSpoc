package queue

import (
	"context"
	"errors"
)

// ErrNoMessage is returned by Consume when the queue had no message ready
// within the blocking window. Callers should treat it as "try again", not
// a failure.
var ErrNoMessage = errors.New("queue: no message available")

type Job struct {
	SubmissionID int64
}

type Message struct {
	// ID is the queue-native message id, needed to Ack/Nack the message.
	ID  string
	Job Job
}

type Queue interface {
	Enqueue(ctx context.Context, job Job) error
	Consume(ctx context.Context, consumer string) (Message, error)
	Ack(ctx context.Context, msg Message) error
	Nack(ctx context.Context, msg Message) error
}
