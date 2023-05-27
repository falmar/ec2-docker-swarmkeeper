package ec2metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

var (
	ErrNotFound             = errors.New("not found")
	ErrUnexpectedStatusCode = errors.New("unexpected status code")
)

var _ Service = &service{}

type Service interface {
	GetToken(ctx context.Context) (string, error)
	GetInstanceInfo(ctx context.Context, token string) (*InstanceInfo, error)
	GetASGReBalance(ctx context.Context, token string) (*ASGReBalanceResponse, error)
	GetSpotInterruption(ctx context.Context, token string) (*SpotInterruptionResponse, error)
	GetLifecycle(ctx context.Context, token string) (*LifecycleResponse, error)
}

func DefaultConfig() *MetadataServiceConfig {
	return &MetadataServiceConfig{
		TokenTTL: 21600,
		Host:     "169.254.169.254",
		Port:     "80",
	}
}

type MetadataServiceConfig struct {
	TokenTTL int
	Host     string
	Port     string
}

func NewService(cfg *MetadataServiceConfig) Service {
	return &service{
		ttl:  cfg.TokenTTL,
		host: cfg.Host,
		port: cfg.Port,
	}
}

type service struct {
	ttl  int
	host string
	port string

	token         string
	lastFetchTime int64
	mu            sync.RWMutex
}

func (t *service) GetToken(ctx context.Context) (string, error) {
	t.mu.Lock()

	if t.token != "" && t.lastFetchTime+int64(t.ttl) > time.Now().Unix() {
		t.mu.Unlock()
		return t.token, nil
	}

	endpoint, err := url.Parse(fmt.Sprintf("http://%s:%s/latest/api/token", t.host, t.port))

	header := http.Header{}
	header.Set("X-aws-ec2-metadata-token-ttl-seconds", strconv.Itoa(t.ttl))
	req := &http.Request{
		Method: "PUT",
		URL:    endpoint,
		Header: header,
	}
	req = req.WithContext(ctx)

	client := &http.Client{Timeout: 5 * time.Second}

	res, err := client.Do(req)
	if err != nil {
		t.mu.Unlock()
		return "", err
	}

	defer res.Body.Close()

	if res.StatusCode == 404 {
		return "", fmt.Errorf("%w: token", ErrNotFound)
	} else if res.StatusCode != 200 {
		return "", fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}

	buf := make([]byte, 256)

	n, err := res.Body.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		t.mu.Unlock()
		return "", err
	}

	token := string(buf[:n])
	t.token = token
	t.lastFetchTime = time.Now().Unix()
	t.mu.Unlock()

	return token, nil
}

type InstanceInfo struct {
	InstanceID       string `json:"instance_id"`
	AutoscalingGroup string `json:"autoscaling_group"`
	LifecycleHook    string `json:"lifecycle_hook"`
}

func (t *service) GetInstanceInfo(ctx context.Context, token string) (*InstanceInfo, error) {
	info := &InstanceInfo{}

	// instance id
	b, err := t.getMeta(ctx, token, "/instance-id")
	if err != nil {
		return nil, err
	}

	info.InstanceID = string(b)

	// asg group name from tag
	b, err = t.getMeta(ctx, token, "/tags/instance/aws:autoscaling:groupName")
	if err != nil {
		return nil, err
	}

	info.AutoscalingGroup = string(b)

	// lifecycle hook name
	b, err = t.getMeta(ctx, token, "/tags/instance/LifecycleHookName")
	if err != nil {
		return nil, err
	}

	info.LifecycleHook = string(b)

	return info, nil
}

type ASGReBalanceResponse struct {
	Time time.Time `json:"noticeTime"`
}

func (t *service) GetASGReBalance(ctx context.Context, token string) (*ASGReBalanceResponse, error) {
	b, err := t.getMeta(ctx, token, "/events/recommendations/rebalance")
	if err != nil {
		return nil, err
	}

	balance := &ASGReBalanceResponse{}

	err = json.Unmarshal(b, balance)
	if err != nil {
		return nil, err
	}

	return balance, nil
}

type SpotInterruptionResponse struct {
	Action string    `json:"action"`
	Time   time.Time `json:"time"`
}

func (t *service) GetSpotInterruption(ctx context.Context, token string) (*SpotInterruptionResponse, error) {
	b, err := t.getMeta(ctx, token, "/spot/instance-action")
	if err != nil {
		return nil, err
	}

	interruption := &SpotInterruptionResponse{}

	err = json.Unmarshal(b, interruption)
	if err != nil {
		return nil, err
	}

	return interruption, nil
}

type LifecycleResponse struct {
	State string `json:"state"`
}

func (t *service) GetLifecycle(ctx context.Context, token string) (*LifecycleResponse, error) {
	b, err := t.getMeta(ctx, token, "/autoscaling/target-lifecycle-state")
	if err != nil {
		return nil, err
	}

	lifecycle := &LifecycleResponse{
		State: string(b),
	}

	return lifecycle, nil
}

func (t *service) getMeta(ctx context.Context, token, path string) ([]byte, error) {
	endpoint, _ := url.Parse(fmt.Sprintf("http://%s:%s/latest/meta-data%s", t.host, t.port, path))
	header := http.Header{}
	header.Set("X-aws-ec2-metadata-token", token)
	client := &http.Client{Timeout: 1 * time.Second}
	req := &http.Request{
		Method: "GET",
		URL:    endpoint,
		Header: header,
	}
	req = req.WithContext(ctx)

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode == 404 {
		return nil, ErrNotFound
	} else if res.StatusCode != 200 {
		return nil, fmt.Errorf("%w: %d ", ErrUnexpectedStatusCode, res.StatusCode)
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	return b, nil
}
