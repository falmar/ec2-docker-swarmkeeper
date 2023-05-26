package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/falmar/docker-swarm-ec2-housekeep/internal/old/types/swarm"
	"io"
	"net/http"
	"net/url"
)

type NodeService interface {
	Info()

	List()
	Inspect()
	Update()
	Remove()
}

type NodeInfoInput struct {
}

type NodeInfoOutput struct {
	NodeId           string          `json:"NodeID"`
	NodeAddr         string          `json:"NodeAddr"`
	LocalNodeState   swarm.NodeState `json:"LocalNodeState"`
	ControlAvailable bool            `json:"ControlAvailable"`
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
