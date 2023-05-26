package worker

import (
	"context"
	"fmt"
	"github.com/docker/docker/client"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/docker"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/ec2metadata"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/slack"
	node "github.com/falmar/ec2-docker-swarmkeeper/internal/worker"
	"github.com/spf13/cobra"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Worker node",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			sigChan := make(chan os.Signal)

			signal.Notify(sigChan, syscall.SIGINT)
			signal.Notify(sigChan, syscall.SIGTERM)

			log.Println("Starting...")

			metadata := ec2metadata.NewService(ec2metadata.DefaultConfig())

			dockerd, err := client.NewClientWithOpts(
				client.WithTimeout(5*time.Second),
				client.WithVersion("v1.42"),
				client.WithHTTPClient(docker.NewSocketClient()),
			)
			if err != nil {
				return fmt.Errorf("failed to create docker client: %s\n", err)
			}

			info, err := dockerd.Ping(ctx)
			if err != nil {
				return fmt.Errorf("failed to ping docker: %s\n", err)
			}

			if info.SwarmStatus == nil || info.SwarmStatus.NodeState == "inactive" {
				return fmt.Errorf("this node is not part of a swarm")
			}

			if info.SwarmStatus.ControlAvailable {
				return fmt.Errorf("this is a manager node, not a worker node")
			}

			worker := node.NewWorker(&node.Config{
				DockerClient:   dockerd,
				EC2Metadata:    metadata,
				ListenInterval: 5 * time.Second,
			})

			go func() {
				<-sigChan
				log.Println("Received quit signal...")
				slack.Notify("Received quit signal...")

				// Allow time for the worker node fetch from metadata service
				time.Sleep(30 * time.Second)

				cancel()
			}()

			log.Println("Started...")

			if err := worker.Listen(ctx); err != nil {
				slack.Notify(fmt.Sprintf("worker node error: %s", err))
				return fmt.Errorf("failed to listen: %s\n", err)
			}
			cancel()

			slack.Notify("Shutting down...")
			log.Println("Shutting down...")

			return nil
		},
	}

	return cmd
}
