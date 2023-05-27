package worker

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/ec2metadata"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/queue"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/slack"
	"log"
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
	interruptChan chan *ec2metadata.SpotInterruptionResponse
	reBalanceChan chan *ec2metadata.ASGReBalanceResponse
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
		interruptChan: make(chan *ec2metadata.SpotInterruptionResponse),
		reBalanceChan: make(chan *ec2metadata.ASGReBalanceResponse),
	}
}

func (w *service) Listen(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go w.handleSpotInterruption(ctx)
	go w.handleASGReBalance(ctx)

breakLoop:
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-w.errChan:
			return err
		case r := <-w.reBalanceChan:
			message := "ASG ReBalance - TS:" + r.Time.Format(time.RFC3339)
			log.Println(message)

			payload, err := json.Marshal(&queue.NodeShutdownPayload{
				NodeID:       w.nodeID,
				InstanceInfo: w.instanceInfo,
				Reason:       message,
			})
			if err != nil {
				return err
			}

			slack.Notify(message)

			id := sha1.Sum(payload)
			err = w.queue.Push(ctx, &queue.Event{
				ID:   hex.EncodeToString(id[:]),
				Name: queue.NodeShutdownEvent,
				Data: payload,
			})
			if err != nil {
				return err
			}

			break breakLoop
		case i := <-w.interruptChan:
			message := "Spot Interruption - TS:" + i.Time.Format(time.RFC3339)
			log.Println(message)

			payload, err := json.Marshal(&queue.NodeShutdownPayload{
				NodeID:       w.nodeID,
				InstanceInfo: w.instanceInfo,

				Reason: message,
			})
			if err != nil {
				return err
			}

			slack.Notify(message)

			id := sha1.Sum(payload)
			err = w.queue.Push(ctx, &queue.Event{
				ID:   hex.EncodeToString(id[:]),
				Name: queue.NodeShutdownEvent,
				Data: payload,
			})
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

			w.interruptChan <- interruption

			return
		}
	}
}

func (w *service) handleASGReBalance(ctx context.Context) {
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

			w.reBalanceChan <- reBalance

			return
		}
	}
}
