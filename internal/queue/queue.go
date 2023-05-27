package queue

import "context"

type EventName string

const (
	NodeShutdownEvent EventName = "node.shutdown"
	NodeRemoveEvent   EventName = "node.remove"
)

type NodeShutdownPayload struct {
	// Docker
	NodeID string `json:"node_id"`
	Reason string `json:"reason"`

	// EC2
	InstanceID string `json:"instance_id"`
}
type NodeRemovePayload struct {
	// Docker
	NodeID string `json:"node_id"`
}

type Event struct {
	ID   string
	Name EventName
	Data interface{}
}

type Queue interface {
	Pop(ctx context.Context, size int64) ([]*Event, error)

	Push(ctx context.Context, event *Event) error
	Retry(ctx context.Context, event *Event) error
	Remove(ctx context.Context, event *Event) error
}
