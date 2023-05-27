package manager

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/queue"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/slack"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/worker"
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
	DrainQueue  queue.Queue
	RemoveQueue queue.Queue
	Dockerd     *client.Client
	AGSClient   *autoscaling.Client
}

type service struct {
	drain   queue.Queue
	remove  queue.Queue
	dockerd *client.Client
	asg     *autoscaling.Client
}

func New(cfg Config) Service {
	return &service{
		drain:   cfg.DrainQueue,
		remove:  cfg.RemoveQueue,
		dockerd: cfg.Dockerd,
		asg:     cfg.AGSClient,
	}
}

func (svc *service) Listen(ctx context.Context) error {
	removeTimer := time.NewTimer(5 * time.Minute)
	drainTimer := time.NewTimer(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-removeTimer.C:
			log.Println("polling for remove events...")
			removeTimer.Reset(5 * time.Minute)

			events, err := svc.remove.Pop(ctx, 10)
			if errors.Is(err, context.Canceled) {
				return nil
			} else if err != nil {
				return err
			}

			var completedEvents []*queue.Event
			for _, event := range events {
				switch event.Name {
				case worker.NodeRemoveEvent:
					// remove the node from the swarm
					err := svc.handleNodeRemoveEvent(ctx, event)
					if err != nil {
						if event.RetryCount > 3 {
							log.Printf("failed to handle node shutdown event: %v", err)
							completedEvents = append(completedEvents, event)
						}

						log.Printf("failed to handle node remove event: %v", err)
						continue
					}

					completedEvents = append(completedEvents, event)
				}
			}

			// complete the events
			for _, event := range completedEvents {
				err := svc.remove.Remove(ctx, event)
				if err != nil {
					log.Printf("failed to remove [remove] event: %v", err)
					continue
				}
			}

			if len(events) != len(completedEvents) {
				log.Printf("failed to complete all events: processed: %d / received: %d", len(completedEvents), len(events))
				continue
			}
		case <-drainTimer.C:
			drainTimer.Reset(30 * time.Second)
			log.Println("polling for drain events...")

			events, err := svc.drain.Pop(ctx, 10)
			// handle empty drain
			if errors.Is(err, context.Canceled) {
				return nil
			} else if err != nil {
				return err
			}

			var completedEvents []*queue.Event
			for _, event := range events {
				switch event.Name {
				case worker.NodeShutdownEvent:
					// drain the node
					err := svc.handleNodeShutdownEvent(ctx, event)
					if err != nil {
						if event.RetryCount > 3 {
							log.Printf("failed to handle node shutdown event: %v", err)
							completedEvents = append(completedEvents, event)
						}

						log.Printf("failed to handle node shutdown event: %v", err)
						continue
					}

					completedEvents = append(completedEvents, event)
				}
			}

			for _, event := range completedEvents {
				err := svc.drain.Remove(ctx, event)
				if err != nil {
					log.Printf("failed to remove [drain] event: %v", err)
					continue
				}
			}

			if len(events) != len(completedEvents) {
				log.Printf("failed to complete all events: processed: %d / received: %d", len(completedEvents), len(events))
				continue
			}
		}
	}

	return nil
}

func (svc *service) handleNodeShutdownEvent(ctx context.Context, event *queue.Event) error {
	slack.Notify(fmt.Sprintf("event [%s]: received node shutdown event", event.ID))

	payload := &worker.NodeShutdownPayload{}

	err := json.Unmarshal(event.Data, &payload)
	if err != nil {
		return fmt.Errorf("event [%s]: failed to unmarshal event data: %w", event.ID, err)
	}

	slack.Notify(fmt.Sprintf("event [%s]: instance id: %s", event.ID, payload.InstanceInfo.InstanceID))

	// record the lifecycle action heartbeat
	if os.Getenv("DEBUG") == "" {
		_, err = svc.asg.RecordLifecycleActionHeartbeat(ctx, &autoscaling.RecordLifecycleActionHeartbeatInput{
			InstanceId:           aws.String(payload.InstanceInfo.InstanceID),
			AutoScalingGroupName: aws.String(payload.InstanceInfo.AutoscalingGroup),
			LifecycleHookName:    aws.String(payload.InstanceInfo.LifecycleHook),
		})
		if err != nil {
			return fmt.Errorf("event [%s]: failed to record lifecycle action heartbeat: %w", event.ID, err)
		}
	}

	node, _, err := svc.dockerd.NodeInspectWithRaw(ctx, payload.NodeID)
	if err != nil {
		return fmt.Errorf("event [%s]: failed to inspect node: %w", event.ID, err)
	}

	node.Spec.Availability = swarm.NodeAvailabilityDrain

	// drain the node
	err = svc.dockerd.NodeUpdate(ctx, node.ID, node.Version, node.Spec)
	if err != nil {
		return fmt.Errorf("event [%s]: failed to drain node: %w", event.ID, err)
	}

	slack.Notify(fmt.Sprintf("event [%s]: draining node: %s", event.ID, node.ID))

	// TODO: monitor the node until it is drained?
	// TODO: delay draining depending on type of interruption?

	// not in production, so don't complete the lifecycle action
	if os.Getenv("DEBUG") == "" {
		_, err = svc.asg.CompleteLifecycleAction(ctx, &autoscaling.CompleteLifecycleActionInput{
			InstanceId:           aws.String(payload.InstanceInfo.InstanceID),
			AutoScalingGroupName: aws.String(payload.InstanceInfo.AutoscalingGroup),
			LifecycleHookName:    aws.String(payload.InstanceInfo.LifecycleHook),

			LifecycleActionResult: aws.String("CONTINUE"),
		})
		if err != nil {
			return fmt.Errorf("event [%s]: failed to complete lifecycle action: %w", event.ID, err)
		}

		slack.Notify(fmt.Sprintf("event [%s]: completed lifecycle action for instance: %s", event.ID, payload.InstanceInfo.InstanceID))
	}

	// "delayed" by X time to allow the node to drain
	data, _ := json.Marshal(&worker.NodeRemovePayload{
		NodeID: payload.NodeID,
	})
	err = svc.remove.Push(ctx, &queue.Event{
		ID:   fmt.Sprintf("%x", sha1.Sum(data)),
		Name: worker.NodeRemoveEvent,
		Data: data,
	}, 0)

	return err
}

func (svc *service) handleNodeRemoveEvent(ctx context.Context, event *queue.Event) error {
	slack.Notify(fmt.Sprintf("event [%s]: received node remove event", event.ID))

	payload := &worker.NodeRemovePayload{}

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

	slack.Notify(fmt.Sprintf("event [%s]: removed node: %s", event.ID, node.ID))

	return nil
}
