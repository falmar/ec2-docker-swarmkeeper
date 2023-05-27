package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/queue"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/slack"
	"log"
	"os"
	"time"
)

var _ Service = (*service)(nil)

// listen for events
// drain the worker node
// complete the lifecycle action for that instance
// add event to remove node from swarm
// remove node from swarm

type Service interface {
	Listen(ctx context.Context) error
}

type Config struct {
	Queue     queue.Queue
	Dockerd   *client.Client
	AGSClient *autoscaling.Client
}

type service struct {
	queue   queue.Queue
	dockerd *client.Client
	asg     *autoscaling.Client
}

func New(cfg Config) Service {
	return &service{
		queue:   cfg.Queue,
		dockerd: cfg.Dockerd,
		asg:     cfg.AGSClient,
	}
}

func (svc *service) Listen(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			log.Println("polling for events...")

			events, err := svc.queue.Pop(ctx, 10)
			// handle empty queue
			if err != nil {
				return err
			}

			var completedEvents []*queue.Event

			for _, event := range events {
				switch event.Name {
				case queue.NodeShutdownEvent:
					// drain the node

					err := svc.handleNodeShutdownEvent(ctx, event)
					if err != nil {
						log.Printf("failed to handle node shutdown event: %v", err)
						continue
					}

					completedEvents = append(completedEvents, event)
				case queue.NodeRemoveEvent:
					// remove the node from the swarm

					err := svc.handleNodeRemoveEvent(ctx, event)
					if err != nil {
						log.Printf("failed to handle node remove event: %v", err)
						continue
					}

					completedEvents = append(completedEvents, event)
				}
			}

			for _, event := range completedEvents {
				err := svc.queue.Remove(ctx, event)
				if err != nil {
					log.Printf("failed to remove event: %v", err)
					continue
				}
			}

			log.Println("sleeping for 10 seconds...")
			<-time.After(10 * time.Second)
		}
	}

	return nil
}

func (svc *service) handleNodeShutdownEvent(ctx context.Context, event *queue.Event) error {
	slack.Notify("received node shutdown event")

	payload := &queue.NodeShutdownPayload{}

	err := json.Unmarshal(event.Data, &payload)
	if err != nil {
		return fmt.Errorf("failed to unmarshal event data: %w", err)
	}

	slack.Notify(fmt.Sprintf("instance id: %s", payload.InstanceInfo.InstanceID))

	// record the lifecycle action heartbeat
	if os.Getenv("DEBUG") == "" {
		_, err = svc.asg.RecordLifecycleActionHeartbeat(ctx, &autoscaling.RecordLifecycleActionHeartbeatInput{
			InstanceId:           aws.String(payload.InstanceInfo.InstanceID),
			AutoScalingGroupName: aws.String(payload.InstanceInfo.AutoscalingGroup),
			LifecycleHookName:    aws.String(payload.InstanceInfo.LifecycleHook),
		})
		if err != nil {
			return fmt.Errorf("failed to record lifecycle action heartbeat: %w", err)
		}
	}

	node, _, err := svc.dockerd.NodeInspectWithRaw(ctx, payload.NodeID)
	if err != nil {
		return fmt.Errorf("failed to inspect node: %w", err)
	}

	node.Spec.Availability = swarm.NodeAvailabilityDrain

	// drain the node
	err = svc.dockerd.NodeUpdate(ctx, node.ID, node.Version, node.Spec)
	if err != nil {
		return fmt.Errorf("failed to drain node: %w", err)
	}

	slack.Notify(fmt.Sprintf("draining node: %s", node.ID))

	// TODO: monitor the node until it is drained?

	// not in production, so don't complete the lifecycle action
	if os.Getenv("DEBUG") == "" {
		_, err = svc.asg.CompleteLifecycleAction(ctx, &autoscaling.CompleteLifecycleActionInput{
			InstanceId:           aws.String(payload.InstanceInfo.InstanceID),
			AutoScalingGroupName: aws.String(payload.InstanceInfo.AutoscalingGroup),
			LifecycleHookName:    aws.String(payload.InstanceInfo.LifecycleHook),

			LifecycleActionResult: aws.String("CONTINUE"),
		})
		if err != nil {
			return fmt.Errorf("failed to complete lifecycle action: %w", err)
		}

		slack.Notify(fmt.Sprintf("completed lifecycle action for instance: %s", payload.InstanceInfo.InstanceID))
	}

	return nil
}

func (svc *service) handleNodeRemoveEvent(ctx context.Context, event *queue.Event) error {
	payload := &queue.NodeRemovePayload{}

	err := json.Unmarshal(event.Data, &payload)
	if err != nil {
		return fmt.Errorf("failed to unmarshal event data: %w", err)
	}

	node, _, err := svc.dockerd.NodeInspectWithRaw(ctx, payload.NodeID)
	if err != nil {
		return fmt.Errorf("failed to inspect node: %w", err)
	}

	err = svc.dockerd.NodeRemove(ctx, node.ID, types.NodeRemoveOptions{
		Force: true,
	})
	if err != nil {
		return fmt.Errorf("failed to remove node: %w", err)
	}

	return nil
}
