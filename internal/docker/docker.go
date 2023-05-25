package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
)

type Docker interface {
	Ping() error
	NodeUpdateAvailability(string, string) error
	NodeRemove(string) error
	LeaveSwarm() error
}

func NewClient() Docker {
	return &docker{client: getDockerClient()}
}

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

func (d *docker) NodeUpdateAvailability(nodeID, availability string) error {
	b, err := json.Marshal(map[string]interface{}{
		"Availability": availability,
	})
	if err != nil {
		return err
	}

	req := &http.Request{
		Method: "POST",
		URL: &url.URL{
			Host:   "localhost",
			Scheme: "http",
			Path:   fmt.Sprintf("/v1.42/nodes/%s/update", nodeID),
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

	return fmt.Errorf("unable to update node availability: %s", c.Message)
}

func (d *docker) NodeRemove(nodeID string) error {
	req := &http.Request{
		Method: "DELETE",
		URL: &url.URL{
			Host:   "localhost",
			Scheme: "http",
			Path:   fmt.Sprintf("/v1.42/nodes/%s", nodeID),
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

	return fmt.Errorf("unable to remove node: %s", c.Message)
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

func getDockerClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", "/var/run/docker.sock")
			},
		},
	}
}
