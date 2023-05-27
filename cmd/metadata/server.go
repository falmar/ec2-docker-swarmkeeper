package metadata

import (
	"context"
	"errors"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/mock_c2metadata"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/slack"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func getHostname() (string, error) {
	f, err := os.Open("/etc/hostname")
	if err != nil {
		return "", err
	}
	defer f.Close()

	b := make([]byte, 128)
	n, err := f.Read(b)
	if err != nil {
		return "", err
	}

	return string(b[:n]), nil
}

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "metadata",
		Short: "EC2 Metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			hostname, err := getHostname()
			if err != nil {
				return err
			}

			server := mock_c2metadata.NewServer(&mock_c2metadata.Config{
				InstanceID: hostname,
				Port:       viper.GetString("metadata.port"),
			})

			go func() {
				sigChan := make(chan os.Signal)
				signal.Notify(sigChan, syscall.SIGINT)
				signal.Notify(sigChan, syscall.SIGTERM)

				<-sigChan
				log.Println("Received quit signal...")
				slack.Notify("Received quit signal...")

				server.Shutdown(ctx)

				cancel()
			}()

			log.Printf("Listening on port %s\n", viper.GetString("metadata.port"))

			err = server.ListenAndServe()
			if !errors.Is(err, http.ErrServerClosed) {
				return err
			}

			log.Println("Shutting down...")

			return nil
		},
	}
}
