package slack

import (
	"fmt"
	"github.com/spf13/viper"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// TODO: make a service out of this
func Notify(text string) {
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
