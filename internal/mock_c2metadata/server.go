package mock_c2metadata

import (
	"encoding/json"
	"fmt"
	"github.com/falmar/ec2-docker-swarmkeeper/internal/ec2metadata"
	"github.com/go-chi/chi/v5"
	"log"
	"net/http"
	"sync"
	"time"
)

type Config struct {
	InstanceID string
	Port       string
}

type server struct {
	instanceID       string
	spotInterruption bool
	rebalance        bool

	mu sync.RWMutex
}

func NewServer(cfg *Config) *http.Server {
	router := chi.NewRouter()

	server := &server{
		instanceID: cfg.InstanceID,
		mu:         sync.RWMutex{},
	}

	router.Put("/latest/api/token", handleGetToken)
	router.Get("/latest/meta-data/instance-id", server.handleInstanceId)
	router.Get("/latest/meta-data/spot/instance-action", server.handleSpotInterruption)
	router.Get("/latest/meta-data/events/recommendations/rebalance", server.handleASGReBalance)

	router.Post("/spot-interruption", server.mockSpotInterruption)
	router.Post("/asg-rebalance", server.mockASGReBalance)

	return &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}
}

func handleGetToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("token"))
}

func (s *server) handleInstanceId(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("x-aws-ec2-metadata-token") == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)

	w.Write([]byte(s.instanceID))
}

func (s *server) handleASGReBalance(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	mock := s.rebalance
	s.mu.RUnlock()

	if r.Header.Get("X-aws-ec2-metadata-token") == "" || !mock {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	err := json.NewEncoder(w).Encode(ec2metadata.ASGReBalanceResponse{
		Time: time.Now(),
	})
	if err != nil {
		log.Println("handle asg re-balance error:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *server) handleSpotInterruption(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	mock := s.spotInterruption
	s.mu.RUnlock()

	token := r.Header.Get("x-aws-ec2-metadata-token")
	fmt.Println("handleSpotInterruption", mock, token)

	if token == "" || !mock {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	err := json.NewEncoder(w).Encode(ec2metadata.SpotInterruptionResponse{
		Action: "terminate",
		Time:   time.Now().Add(1 * time.Minute),
	})
	if err != nil {
		log.Println("handle spot interruption error:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *server) mockSpotInterruption(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.spotInterruption = true
	s.mu.Unlock()
}

func (s *server) mockASGReBalance(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.rebalance = true
	s.mu.Unlock()
}
