package worker

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/ec2metadata"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/queue"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/slack"
	"time"
)

type Service interface {
	Listen(ctx context.Context) error
}

type service struct {
	nodeID     string
	instanceID string
	metadata   ec2metadata.Service
	interval   time.Duration
	queue      queue.Queue

	errChan       chan error
	interruptChan chan *ec2metadata.SpotInterruptionResponse
	reBalanceChan chan *ec2metadata.ASGReBalanceResponse
}

type Config struct {
	NodeID     string
	InstanceID string

	Queue          queue.Queue
	EC2Metadata    ec2metadata.Service
	ListenInterval time.Duration
}

func NewWorker(cfg *Config) Service {
	return &service{
		nodeID:     cfg.NodeID,
		instanceID: cfg.InstanceID,

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
			cancel()

			payload, err := json.Marshal(&queue.NodeShutdownPayload{
				NodeID:     w.nodeID,
				InstanceID: w.instanceID,
				Reason:     "ASG ReBalance - TS:" + r.Time.Format(time.RFC3339),
			})
			if err != nil {
				return err
			}

			slack.Notify("ASG Rebalance")
			err = w.queue.Push(ctx, &queue.Event{
				Name: queue.NodeShutdownEvent,
				Data: payload,
			})
			if err != nil {
				return err
			}

			break breakLoop
		case i := <-w.interruptChan:
			cancel()
			cancel()

			payload, err := json.Marshal(&queue.NodeShutdownPayload{
				NodeID:     w.nodeID,
				InstanceID: w.instanceID,
				Reason:     "ASG ReBalance - TS:" + i.Time.Format(time.RFC3339),
			})
			if err != nil {
				return err
			}

			slack.Notify("Spot Interruption")
			err = w.queue.Push(ctx, &queue.Event{
				Name: queue.NodeShutdownEvent,
				Data: payload,
			})
			if err != nil {
				return err
			}

			break breakLoop
		}
	}

	return nil
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

			interruption, err := w.metadata.GetSpotInterruption(ctx, token)
			if errors.Is(err, ec2metadata.ErrInterruptionNotFound) {
				continue
			} else if err != nil {
				w.errChan <- err
				return
			}

			w.interruptChan <- interruption
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

			reBalance, err := w.metadata.GetASGReBalance(ctx, token)
			if errors.Is(err, ec2metadata.ErrRebalanceNotFount) {
				continue
			} else if err != nil {
				w.errChan <- err
				return
			}

			w.reBalanceChan <- reBalance
		}
	}
}
