package swarm

type NodeState string

const (
	NodeStateUnknown      NodeState = "unknown"
	NodeStateDown         NodeState = "down"
	NodeStateReady        NodeState = "ready"
	NodeStateDisconnected NodeState = "disconnected"
)

type NodeAvailability string

const (
	NodeAvailabilityActive NodeAvailability = "active"
	NodeAvailabilityPause  NodeAvailability = "pause"
	NodeAvailabilityDrain  NodeAvailability = "drain"
)

type NodeRole string

const (
	NodeRoleManager NodeRole = "manager"
	NodeRoleWorker  NodeRole = "worker"
)

type NodeSpec struct {
	Name         string            `json:"Name"`
	Labels       map[string]string `json:"Labels"`
	Role         NodeRole          `json:"Role"`
	Availability NodeAvailability  `json:"Availability"`
}

type NodeStatus struct {
	State   NodeState `json:"State"`
	Message string    `json:"Message"`
}
