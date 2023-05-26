package worker

import (
	"context"
	"errors"
	"github.com/docker/docker/client"
	"github.com/falmar/docker-swarm-ec2-housekeep/internal/ec2metadata"
	"github.com/falmar/docker-swarm-ec2-housekeep/internal/slack"
	"time"
)

type Worker interface {
	Listen(ctx context.Context) error
}

type worker struct {
	dockerClient *client.Client
	metadata     ec2metadata.Service
	interval     time.Duration

	errChan       chan error
	interruptChan chan *ec2metadata.SpotInterruptionResponse
	reBalanceChan chan *ec2metadata.ASGReBalanceResponse
}

type Config struct {
	DockerClient   *client.Client
	EC2Metadata    ec2metadata.Service
	ListenInterval time.Duration
}

func NewWorker(cfg *Config) Worker {
	return &worker{
		dockerClient: cfg.DockerClient,
		metadata:     cfg.EC2Metadata,
		interval:     cfg.ListenInterval,

		errChan:       make(chan error),
		interruptChan: make(chan *ec2metadata.SpotInterruptionResponse),
		reBalanceChan: make(chan *ec2metadata.ASGReBalanceResponse),
	}
}

func (w *worker) Listen(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go w.handleSpotInterruption(ctx)
	go w.handleASGReBalance(ctx)

breakLoop:
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-w.reBalanceChan:
			cancel()
			slack.Notify("ASG Rebalance")
			// notify the manager
			// drain the worker node
			// complete the lifecycle action
			break breakLoop
		case <-w.interruptChan:
			cancel()
			slack.Notify("Spot Interruption")
			// notify the manager
			// drain the worker node
			break breakLoop
		}
	}

	return nil
}

func (w *worker) handleSpotInterruption(ctx context.Context) {
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

func (w *worker) handleASGReBalance(ctx context.Context) {
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
