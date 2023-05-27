package manager

import (
	"github.com/falmar/ec2-docker-swarmkeeper/internal/queue"
)

// listen for events
// drain the worker node
// complete the lifecycle action for that instance
// add event to remove node from swarm
// remove node from swarm

type Service interface {
	Listen() error
}

type Config struct {
	Queue queue.Queue
}

type service struct {
	queue queue.Queue
}
