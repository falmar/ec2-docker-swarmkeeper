package ec2instance

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
	ErrTokenNotFound        = errors.New("token not found")
	ErrRebalanceNotFount    = errors.New("rebalance not found")
	ErrInterruptionNotFound = errors.New("spot interruption not found")
)

var _ MetadataService = &tokenService{}

type MetadataService interface {
	GetToken(ctx context.Context) (string, error)
	GetASGReBalance(ctx context.Context, token string) (*ASGReBalanceResponse, error)
	GetSpotInterruption(ctx context.Context, token string) (*SpotInterruptionResponse, error)
}

func NewMetadataService(tokenTTL int) MetadataService {
	return &tokenService{
		ttl: tokenTTL,
	}
}

type tokenService struct {
	ttl           int
	token         string
	lastFetchTime int64
	mu            sync.RWMutex
}

func (t *tokenService) GetToken(ctx context.Context) (string, error) {
	t.mu.Lock()

	if t.token == "" || t.lastFetchTime+int64(t.ttl) < time.Now().Unix() {
		t.mu.Unlock()
		return t.token, nil
	}

	endpoint, _ := url.Parse("http://169.254.169.254/latest/api/token")
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
		return "", ErrTokenNotFound
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

type ASGReBalanceResponse struct {
	Time time.Time
}

func (t *tokenService) GetASGReBalance(ctx context.Context, token string) (*ASGReBalanceResponse, error) {
	endpoint, _ := url.Parse("http://169.254.169.254/latest/api/token")
	header := http.Header{}
	header.Set("X-aws-ec2-metadata-token", token)
	client := &http.Client{Timeout: 1 * time.Second}
	req := &http.Request{
		Method: "PUT",
		URL:    endpoint,
		Header: header,
	}
	req = req.WithContext(ctx)

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode == 404 {
		return nil, ErrRebalanceNotFount
	} else if res.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}

	balance := &ASGReBalanceResponse{}
	err = json.NewDecoder(res.Body).Decode(&balance)
	if err != nil {
		return nil, err
	}

	return balance, nil
}

type SpotInterruptionResponse struct {
	Action string    `json:"action"`
	Time   time.Time `json:"time"`
}

func (t *tokenService) GetSpotInterruption(ctx context.Context, token string) (*SpotInterruptionResponse, error) {
	endpoint, _ := url.Parse("http://169.254.169.254/latest/meta-data/spot/instance-action")
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
	defer res.Body.Close()

	if res.StatusCode == 404 {
		return nil, ErrInterruptionNotFound
	} else if res.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}

	interruption := &SpotInterruptionResponse{}

	err = json.NewDecoder(res.Body).Decode(&interruption)
	if err != nil {
		return nil, err
	}

	return interruption, nil
}
