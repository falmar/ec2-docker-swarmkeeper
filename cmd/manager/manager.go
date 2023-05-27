package manager

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/docker/docker/client"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/docker"
	node "github.com/falmar/ec2-docker-swarmkeeper/internal/manager"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/queue"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/slack"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// --
// TODO: implement raft quorum for:
// 1. leader election, so one service will long poll for events and take action
// --

// --
// This manager command will be responsible for
// 1. draining worker nodes
// 2. removing them from the swarm cluster
// 3. terminating (complete lifecycle hook) the EC2 instance
// --

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "manager",
		Short: "Listen for EC2 Spot Interruption and ASG Rebalance events from worker nodes and take action",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			log.Println("Starting...")

			// dockerd
			dockerd, err := client.NewClientWithOpts(
				client.WithTimeout(5*time.Second),
				client.WithVersion("v1.42"),
				client.WithHTTPClient(docker.NewSocketClient()),
			)
			if err != nil {
				return fmt.Errorf("failed to create docker client: %s\n", err)
			}

			pingOut, err := dockerd.Ping(ctx)
			if err != nil {
				return fmt.Errorf("failed to ping docker: %s\n", err)
			}

			if pingOut.SwarmStatus == nil || pingOut.SwarmStatus.NodeState == "inactive" {
				return fmt.Errorf("this node is not part of a swarm")
			}

			if !pingOut.SwarmStatus.ControlAvailable {
				return fmt.Errorf("this is a worker node, not a manager node")
			}

			//infoOut, err := dockerd.Info(ctx)
			//if err != nil {
			//	return fmt.Errorf("failed to get docker info: %s\n", err)
			//}
			// -- docker

			// aws
			awsConfig, err := config.LoadDefaultConfig(
				ctx,
				config.WithRegion(viper.GetString("aws.region")),
				config.WithCredentialsProvider(
					credentials.NewStaticCredentialsProvider(
						viper.GetString("aws.access_key_id"),
						viper.GetString("aws.secret_access_key"),
						"",
					),
				),
			)
			if err != nil {
				return fmt.Errorf("failed to load aws config: %s\n", err)
			}
			// -- aws

			// ASG
			asg := autoscaling.NewFromConfig(awsConfig)
			// -- ASG

			// queue
			sqsQueue := queue.NewSQSQueue(&queue.SQSConfig{
				QueueURL:          viper.GetString("sqs.queue_url"),
				Client:            sqs.NewFromConfig(awsConfig),
				PollInterval:      20 * time.Second, // this node doesnt poll, it just pushes
				VisibilityTimeout: 1 * time.Minute,
			})
			// -- queue

			svc := node.New(node.Config{
				AGSClient: asg,
				Queue:     sqsQueue,
				Dockerd:   dockerd,
			})

			go func() {
				sigChan := make(chan os.Signal)
				signal.Notify(sigChan, syscall.SIGINT)
				signal.Notify(sigChan, syscall.SIGTERM)

				<-sigChan
				log.Println("Received quit signal...")
				slack.Notify("Received quit signal...")
				cancel()
			}()

			err = svc.Listen(ctx)
			if err != nil {
				slack.Notify(fmt.Sprintf("manager error: %s", err))
				return fmt.Errorf("manager error: %s\n", err)
			}

			slack.Notify("Manager Shutting down...")
			log.Println("Manager shutting down...")

			return err
		},
	}
}
