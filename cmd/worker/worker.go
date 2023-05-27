package worker

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/docker/docker/client"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/docker"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/ec2metadata"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/queue"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/slack"
	node "github.com/falmar/ec2-docker-swarmkeeper/internal/worker"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Listen for EC2 Spot Interruption and ASG Rebalance events",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			log.Println("Starting...")

			// metadata
			var metaConfig *ec2metadata.MetadataServiceConfig

			if viper.GetString("metadata.host") != "" && viper.GetString("metadata.port") != "" {
				metaConfig = &ec2metadata.MetadataServiceConfig{
					Host: viper.GetString("metadata.host"),
					Port: viper.GetString("metadata.port"),
				}
			} else {
				metaConfig = ec2metadata.DefaultConfig()
			}

			metadata := ec2metadata.NewService(metaConfig)

			token, err := metadata.GetToken(ctx)
			if err != nil {
				return fmt.Errorf("failed to get token: %s\n", err)
			}

			fmt.Println("token: ", token)

			instanceId, err := metadata.GetInstanceId(ctx, token)
			if err != nil {
				return fmt.Errorf("failed to get instance id: %s\n", err)
			}
			// -- metadata

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

			//if pingOut.SwarmStatus.ControlAvailable {
			//	return fmt.Errorf("this is a manager node, not a worker node")
			//}

			infoOut, err := dockerd.Info(ctx)
			if err != nil {
				return fmt.Errorf("failed to get docker info: %s\n", err)
			}
			// -- docker

			// aws
			awsConfig, err := config.LoadDefaultConfig(
				ctx,
				config.WithDefaultRegion(viper.GetString("aws.region")),
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

			sqsQueue := queue.NewSQSQueue(&queue.SQSConfig{
				QueueURL:          viper.GetString("sqs.queue_url"),
				Client:            sqs.NewFromConfig(awsConfig),
				PollInterval:      5 * time.Minute, // this node doesnt poll, it just pushes
				VisibilityTimeout: 0,
			})

			worker := node.NewWorker(&node.Config{
				InstanceID: instanceId,
				NodeID:     infoOut.Swarm.NodeID,
				Queue:      sqsQueue,

				EC2Metadata:    metadata,
				ListenInterval: 5 * time.Second,
			})

			go func() {
				sigChan := make(chan os.Signal)
				signal.Notify(sigChan, syscall.SIGINT)
				signal.Notify(sigChan, syscall.SIGTERM)

				<-sigChan
				log.Println("Received quit signal...")
				slack.Notify("Received quit signal...")

				// Allow time for the worker node fetch from metadata service
				if os.Getenv("DEBUG") == "" {
					time.Sleep(30 * time.Second)
				}

				cancel()
			}()

			log.Println("Started...")

			if err := worker.Listen(ctx); err != nil {
				slack.Notify(fmt.Sprintf("worker node error: %s", err))
				return fmt.Errorf("worker node error: %s\n", err)
			}
			cancel()

			slack.Notify("Shutting down...")
			log.Println("Shutting down...")

			return nil
		},
	}

	return cmd
}
