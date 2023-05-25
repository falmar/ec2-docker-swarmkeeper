package main

import (
	"context"
	"github.com/falmar/docker-swarm-ec2-housekeep/cmd/worker"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	viper.SetConfigFile("config.yaml")
	viper.SetConfigType("yaml")
	viper.ReadInConfig()

	rootCmd := cobra.Command{
		Use:   "docker-swarm-ec2-housekeep",
		Short: "Docker Swarm EC2 Housekeep",
	}

	rootCmd.AddCommand(worker.Cmd())

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		panic(err)
	}
}
