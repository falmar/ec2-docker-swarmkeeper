package worker

import (
	"github.com/falmar/ec2-docker-swarmkeeper/internal/ec2metadata"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/queue"
	"time"
)

const (
	NodeDrainEvent  queue.EventName = "node.drain"
	NodeRemoveEvent queue.EventName = "node.remove"
)

type NodeDrainPayload struct {
	// Docker
	NodeID string `json:"node_id"`
	Reason string `json:"reason"`

	Type InterruptType
	Time time.Time `json:"time"`

	// EC2
	InstanceInfo *ec2metadata.InstanceInfo `json:"instance_info"`
}
type NodeRemovePayload struct {
	// Docker
	NodeID string `json:"node_id"`
}
