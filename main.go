package main

import (
	"fmt"
	"github.com/spf13/viper"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	viper.SetConfigFile("config.yaml")
	viper.SetConfigType("yaml")
	viper.ReadInConfig()

	done := make(chan struct{}, 1)

	sigChan := make(chan os.Signal)

	signal.Notify(sigChan, syscall.SIGINT)
	signal.Notify(sigChan, syscall.SIGTERM)

	log.Println("Started...")

	go func() {
		<-sigChan
		log.Println("Received quit signal...")
		notifySlack()
		done <- struct{}{}
	}()

	<-done

	log.Println("Exiting...")
}

func notifySlack() {
	endpoint, _ := url.Parse("https://slack.com/api/chat.postMessage")
	header := http.Header{}
	header.Set("Content-Type", "application/json")
	header.Set("Authorization", fmt.Sprintf("Bearer %s", viper.GetString("slack.token")))
	message := fmt.Sprintf(`{"channel": "%s", "text": "%s"}`, viper.Get("slack.channel"), "Hello from Go!")

	req := &http.Request{
		Method: "POST",
		URL:    endpoint,
		Header: header,
		Body:   io.NopCloser(strings.NewReader(message)),
	}

	(&http.Client{}).Do(req)
}
