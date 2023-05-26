package worker

import (
	"errors"
	"fmt"
	"github.com/docker/docker/client"
	"github.com/falmar/docker-swarm-ec2-housekeep/internal/ec2instance"
	"github.com/falmar/docker-swarm-ec2-housekeep/internal/slack"
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
			ctx := cmd.Context()

			done := make(chan struct{}, 1)

			sigChan := make(chan os.Signal)

			signal.Notify(sigChan, syscall.SIGINT)
			signal.Notify(sigChan, syscall.SIGTERM)

			log.Println("Started...")

			// 6 hours in sec
			metadata := ec2instance.NewMetadataService(21600)

			dockerd, err := client.NewClientWithOpts()
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

			go func() {
			breakLoop:
				for {
					select {
					case <-ctx.Done():
						break breakLoop
					case <-time.After(5 * time.Second):
						token, err := metadata.GetToken(ctx)
						if err != nil {
							log.Printf("failed to get token... %s\n", err)
						}

						log.Println("Checking for rebalance...")
						rebalance, err := metadata.GetASGReBalance(ctx, token)
						if errors.Is(err, ec2instance.ErrRebalanceNotFount) {
							continue
						} else if err != nil {
							log.Printf("failed to check for rebalance... %s\n", err)
							continue
						}

						message := fmt.Sprintf("Rebalance detected... Time: %s", rebalance.Time)
						log.Println(message)
						slack.Notify(message)

						// Notify Manager nodes we gotta go :'( it was fun

						break breakLoop
					}
				}

				done <- struct{}{}
			}()

			go func() {
			breakLoop:
				for {
					select {
					case <-ctx.Done():
						break breakLoop
					case <-time.After(5 * time.Second):
						token, err := metadata.GetToken(ctx)
						if err != nil {
							log.Printf("failed to get token... %s\n", err)
						}

						log.Println("Checking for spot interruption...")
						interruption, err := metadata.GetSpotInterruption(ctx, token)
						if err != nil {
							log.Printf("Failed to check for spot interruption... %s\n", err)
							continue
						}

						message := fmt.Sprintf("Spot interruption detected... Action: %s; Time: %s", interruption.Action, interruption.Time)
						log.Println(message)
						slack.Notify(message)

						// Notify Manager nodes we gotta go :'( it was fun

						break breakLoop
					}
				}

				done <- struct{}{}
			}()

			go func() {
				<-sigChan
				log.Println("Received quit signal...")
				slack.Notify("Received quit signal...")

				// Allow time for the other goroutines to finish
				time.Sleep(30 * time.Second)

				done <- struct{}{}
			}()

			<-done

			slack.Notify("Shutting down...")
			log.Println("Shutting down...")

			return nil
		},
	}

	return cmd
}
