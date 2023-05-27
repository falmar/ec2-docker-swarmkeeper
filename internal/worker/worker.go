package worker

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/ec2metadata"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/queue"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/slack"
	"log"
	"strings"
	"time"
)

type Service interface {
	Listen(ctx context.Context) error
}

type service struct {
	nodeID       string
	instanceInfo *ec2metadata.InstanceInfo
	metadata     ec2metadata.Service
	interval     time.Duration
	queue        queue.Queue

	errChan       chan error
	interruptChan chan *InstanceInterrupt
}

type Config struct {
	NodeID       string
	InstanceInfo *ec2metadata.InstanceInfo

	Queue          queue.Queue
	EC2Metadata    ec2metadata.Service
	ListenInterval time.Duration
}

func NewWorker(cfg *Config) Service {
	return &service{
		nodeID:       cfg.NodeID,
		instanceInfo: cfg.InstanceInfo,

		metadata: cfg.EC2Metadata,
		interval: cfg.ListenInterval,
		queue:    cfg.Queue,

		errChan:       make(chan error),
		interruptChan: make(chan *InstanceInterrupt),
	}
}

func (w *service) Listen(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go w.handleSpotInterruption(ctx)
	go w.handleASGReBalance(ctx)
	go w.handleLifecycle(ctx)

breakLoop:
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-w.errChan:
			return err
		case i := <-w.interruptChan:
			message := fmt.Sprintf("Interruption [%s] detected at %s", i.Type, time.Now().Format(time.RFC3339))
			log.Println(message)

			payload, err := json.Marshal(&NodeDrainPayload{
				NodeID:       w.nodeID,
				InstanceInfo: w.instanceInfo,

				Type: i.Type,
				Time: i.Time,

				Reason: message,
			})
			if err != nil {
				return err
			}

			slack.Notify(message)

			id := sha1.Sum(payload)
			err = w.queue.Push(ctx, &queue.Event{
				ID:   hex.EncodeToString(id[:]),
				Name: NodeDrainEvent,
				Data: payload,
			}, 0)
			if err != nil {
				return err
			}

			break breakLoop
		}
	}

	// Wait for context to be done
	<-ctx.Done()

	return ctx.Err()
}

func (w *service) handleSpotInterruption(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(w.interval):
			token, err := w.metadata.GetToken(ctx)
			if err != nil {
				w.errChan <- err
				return
			}

			log.Println("checking for spot interruption...")
			interruption, err := w.metadata.GetSpotInterruption(ctx, token)
			if errors.Is(err, ec2metadata.ErrNotFound) {
				continue
			} else if err != nil {
				w.errChan <- err
				return
			}

			w.interruptChan <- &InstanceInterrupt{
				Type: SpotInterrupt,
				Time: interruption.Time,
			}

			return
		}
	}
}

type InterruptType string

const (
	SpotInterrupt         InterruptType = "Spot Interruption"
	ASGReBalanceInterrupt InterruptType = "ASG ReBalance"
	LifecycleInterrupt    InterruptType = "Lifecycle"
)

type InstanceInterrupt struct {
	Type InterruptType
	Time time.Time
}

func (w *service) handleASGReBalance(ctx context.Context) {
	var found int

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(w.interval):
			token, err := w.metadata.GetToken(ctx)
			if err != nil {
				w.errChan <- err
				return
			}

			log.Println("checking for asg re-balance...")
			reBalance, err := w.metadata.GetASGReBalance(ctx, token)
			if errors.Is(err, ec2metadata.ErrNotFound) {
				continue
			} else if err != nil {
				w.errChan <- err
				return
			}

			if found == 0 {
				found++
				// wait for the next iteration see if spot interruption is found
				continue
			}

			w.interruptChan <- &InstanceInterrupt{
				Type: ASGReBalanceInterrupt,
				Time: reBalance.Time,
			}

			return
		}
	}
}

func (w *service) handleLifecycle(ctx context.Context) {
	var terminating int

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(w.interval):
			token, err := w.metadata.GetToken(ctx)
			if err != nil {
				w.errChan <- err
				return
			}

			log.Println("checking for lifecycle...")
			lifecycle, err := w.metadata.GetLifecycle(ctx, token)
			if errors.Is(err, ec2metadata.ErrNotFound) {
				continue
			} else if err != nil {
				w.errChan <- err
				return
			} else if strings.ToLower(lifecycle.State) != "terminated" {
				terminating = 0
				continue
			}

			// if we find a lifecycle event, we wait for the next 2 iteration to see if there is a spot interruption
			if terminating < 2 {
				terminating++
				continue
			}

			w.interruptChan <- &InstanceInterrupt{
				Type: LifecycleInterrupt,
				Time: time.Now(),
			}

			return
		}
	}
}
