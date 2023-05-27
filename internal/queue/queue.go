package queue

import (
	"context"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/ec2metadata"
)

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
	InstanceInfo *ec2metadata.InstanceInfo `json:"instance_info"`
}
type NodeRemovePayload struct {
	// Docker
	NodeID string `json:"node_id"`
}

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
