package worker

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/falmar/docker-swarm-ec2-housekeep/internal/docker"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Worker node",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()

			done := make(chan struct{}, 1)

			sigChan := make(chan os.Signal)

			signal.Notify(sigChan, syscall.SIGINT)
			signal.Notify(sigChan, syscall.SIGTERM)

			log.Println("Started...")

			dkr := docker.NewClient()

			tokenChan := make(chan string)
			leaveChan := make(chan struct{})
			var token string

			err := dkr.Ping()
			if err != nil {
				log.Fatalf("failed to ping docker: %s\n", err)
				return
			}

			go func() {
				var err error = nil

				for {
					token, err = getInstanceToken()
					if err != nil {
						log.Println("Failed to refresh token...")
						log.Println(err)
						<-time.After(5 * time.Second)
						continue
					}

					tokenChan <- token

					<-time.After(6 * time.Hour)
				}
			}()

			go func() {
			breakLoop:
				for {
					select {
					case <-ctx.Done():
						break breakLoop
					case <-time.After(5 * time.Second):
						if token == "" {
							continue
						}

						log.Println("Checking for rebalance...")
						rebalance, err := checkForASGReBalance(token)
						if err != nil {
							log.Printf("failed to check for rebalance... %s\n", err)
						} else if rebalance != nil {
							message := fmt.Sprintf("Rebalance detected... Time: %s", rebalance.Time)

							log.Println(message)
							notifySlack(message)
							leaveChan <- struct{}{}

							break breakLoop
						}

					}
				}
			}()

			go func() {
			breakLoop:
				for {
					select {
					case <-ctx.Done():
						break breakLoop
					case <-time.After(5 * time.Second):
						if token == "" {
							continue
						}

						log.Println("Checking for spot interruption...")
						interruption, err := checkForSpotInterruption(token)
						if err != nil {
							log.Printf("Failed to check for spot interruption... %s\n", err)
						} else if interruption != nil {
							message := fmt.Sprintf("Spot interruption detected... Action: %s; Time: %s", interruption.Action, interruption.Time)

							log.Println(message)
							notifySlack(message)
							leaveChan <- struct{}{}

							break breakLoop
						}
					}
				}
			}()

			go func() {
			breakLoop:
				for {
					select {
					case <-ctx.Done():
						break breakLoop
					case <-time.After(5 * time.Second):
						if token == "" {
							continue
						}

						log.Println("Checking for lifecycle status...")
						status, err := checkForTerminateLifecycleStatus(token)

						if err != nil {
							log.Printf("Failed to check for lifecycle status... %s\n", err)
						} else if status != nil {
							message := fmt.Sprintf("Lifecycle status detected... Status: %s; Time: %s", status.Status, time.Now().Format(time.RFC3339))

							log.Println(message)
							notifySlack(message)

							leaveChan <- struct{}{}

							break breakLoop
						}
					}
				}
			}()

			go func() {
				select {
				case <-leaveChan:
					log.Println("Leaving swarm...")
					notifySlack("Leaving swarm...")
					//err := dkr.LeaveSwarm()
					//if err != nil {
					//	log.Printf("Failed to leave swarm... %s\n", err)
					//}
					notifySlack("Left swarm...")

					done <- struct{}{}
				}
			}()

			go func() {
				<-sigChan
				log.Println("Received quit signal...")
				notifySlack("Received quit signal...")

				// Allow time for the other goroutines to finish
				time.Sleep(30 * time.Second)

				done <- struct{}{}
			}()

			<-done

			notifySlack("Shutting down...")
			log.Println("Shutting down...")
		},
	}

	return cmd
}

func notifySlack(text string) {
	endpoint, _ := url.Parse("https://slack.com/api/chat.postMessage")
	header := http.Header{}
	header.Set("Content-Type", "application/json")
	header.Set("Authorization", fmt.Sprintf("Bearer %s", viper.GetString("slack.token")))
	message := fmt.Sprintf(`{"channel": "%s", "text": "%s"}`, viper.Get("slack.channel"), text)

	req := &http.Request{
		Method: "POST",
		URL:    endpoint,
		Header: header,
		Body:   io.NopCloser(strings.NewReader(message)),
	}

	(&http.Client{}).Do(req)
}

func getInstanceToken() (string, error) {
	endpoint, _ := url.Parse("http://169.254.169.254/latest/api/token")
	header := http.Header{}
	header.Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")
	req := &http.Request{
		Method: "PUT",
		URL:    endpoint,
		Header: header,
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	buf := make([]byte, 256)

	n, err := resp.Body.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	return string(buf[:n]), nil
}

type ASGReBalanceResponse struct {
	Time time.Time `json:"noticeTime"`
}

func checkForASGReBalance(token string) (*ASGReBalanceResponse, error) {
	endpoint, _ := url.Parse("http://169.254.169.254/latest/meta-data/events/recommendations/rebalance")
	header := http.Header{}
	header.Set("X-aws-ec2-metadata-token", token)
	req := &http.Request{
		Method: "GET",
		URL:    endpoint,
		Header: header,
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil
	} else if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	rebalance := &ASGReBalanceResponse{}

	err = json.NewDecoder(resp.Body).Decode(&rebalance)
	if err != nil {
		return nil, err
	}

	return rebalance, nil
}

type SpotInterruptionResponse struct {
	Action string    `json:"action"`
	Time   time.Time `json:"time"`
}

func checkForSpotInterruption(token string) (*SpotInterruptionResponse, error) {
	endpoint, _ := url.Parse("http://169.254.169.254/latest/meta-data/spot/instance-action")
	header := http.Header{}
	header.Set("X-aws-ec2-metadata-token", token)
	req := &http.Request{
		Method: "GET",
		URL:    endpoint,
		Header: header,
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil
	} else if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	interruption := &SpotInterruptionResponse{}

	json.NewDecoder(resp.Body).Decode(&interruption)
	if err != nil {
		return nil, err
	}

	return interruption, nil
}

type LifecycleStatusResponse struct {
	Status string
}

func checkForTerminateLifecycleStatus(token string) (*LifecycleStatusResponse, error) {
	endpoint, _ := url.Parse("http://169.254.169.254/latest/meta-data/autoscaling/target-lifecycle-state")
	header := http.Header{}
	header.Set("X-aws-ec2-metadata-token", token)
	req := &http.Request{
		Method: "GET",
		URL:    endpoint,
		Header: header,
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil
	} else if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	status := &LifecycleStatusResponse{}

	buffer := make([]byte, 256)

	n, err := resp.Body.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	status.Status = strings.ToLower(string(buffer[:n]))

	if status.Status != "terminated" {
		return nil, nil
	}

	return status, nil
}

// TODO: Complete the lifecycle hook