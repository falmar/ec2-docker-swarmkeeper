package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type Client interface {
	Ping() error

	LeaveSwarm() error
}

//func NewClient() Client {
//	return &docker{client: getDockerClient()}
//}

type docker struct {
	client *http.Client
}

func (d *docker) Ping() error {
	req := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Host:   "localhost",
			Scheme: "http",
			Path:   "/v1.42/_ping",
		},
	}

	res, err := d.client.Do(req)
	if err != nil {
		return err
	}

	if res.StatusCode == 200 {
		return nil
	}

	c := struct {
		Message string `json:"message"`
	}{}

	err = json.NewDecoder(res.Body).Decode(&c)
	if err != nil {
		return err
	}

	return fmt.Errorf("unable to ping docker: %s", c.Message)
}

func (d *docker) LeaveSwarm() error {
	b, err := json.Marshal(map[string]interface{}{})
	if err != nil {
		return err
	}

	req := &http.Request{
		Method: "POST",
		URL: &url.URL{
			Host:   "localhost",
			Scheme: "http",
			Path:   "/v1.42/swarm/leave",
		},
		Body: io.NopCloser(bytes.NewReader(b)),
	}

	res, err := d.client.Do(req)
	if err != nil {
		return err
	}

	if res.StatusCode == 200 {
		return nil
	}

	c := struct {
		Message string `json:"message"`
	}{}

	err = json.NewDecoder(res.Body).Decode(&c)
	if err != nil {
		return err
	}

	return fmt.Errorf("unable to leave swarm: %s", c.Message)
}