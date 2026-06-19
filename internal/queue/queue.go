package queue

import "context"

type Job struct {
	SubmissionID int64
}

type Message struct {
	ID string
	Job Job
}

type Queue interface {
	Enqueue(ctx context.Context, job Job)
	Consume(ctx context.Context, consumer string) (Message, error)
	Ack(ctx context.Context, msg Message)
	Nack(ctx context.Context, msg Message)
}