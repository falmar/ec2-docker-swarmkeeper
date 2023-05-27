package main

import (
	"context"
	"github.com/falmar/ec2-docker-swarmkeeper/cmd/metadata"
	"github.com/falmar/ec2-docker-swarmkeeper/cmd/worker"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"log"
)

var rootCmd = cobra.Command{
	Use:   "docker-swarm-ec2-housekeep",
	Short: "Docker Swarm EC2 Housekeep",
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	viper.SetConfigFile("config.yaml")
	viper.SetConfigType("yaml")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("failed to read config: %s\n", err)
	}

	rootCmd.AddCommand(worker.Cmd())
	rootCmd.AddCommand(metadata.Cmd())

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		log.Fatalf("failed to execute command: %s\n", err)
	}
}
