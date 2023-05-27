package queue

import (
	"context"
)

type EventName string

type Event struct {
	sqsReceiptHandle string

	ID   string
	Name EventName
	Data []byte

	RetryCount int
}

type Queue interface {
	Pop(ctx context.Context, size int64) ([]*Event, error)

	Push(ctx context.Context, event *Event, delay int64) error
	Retry(ctx context.Context, event *Event) error
	Remove(ctx context.Context, event *Event) error
}
