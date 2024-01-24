package service

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"
)

// CircuitBreaker states.
const (
	ClosedState = iota
	OpenState
)

var (
	// ErrCircuitOpen indicates that the circuit breaker is open.
	ErrCircuitOpen                        = errors.New("circuit breaker is open")
	ErrUnexpectedCircuitBreakerResultType = errors.New("unexpected result type from circuit breaker")
)

// CircuitBreakerConfig holds the configuration for the CircuitBreaker.
type CircuitBreakerConfig struct {
	Enabled   bool
	MaxRetry  int
	Threshold int
	Timeout   time.Duration
	Interval  time.Duration
	HealthURL string
}

// CircuitBreaker represents a circuit breaker implementation.
type CircuitBreaker struct {
	mu           sync.Mutex
	state        int // ClosedState or OpenState
	failureCount int
	maxRetry     int
	threshold    int
	timeout      time.Duration
	interval     time.Duration
	healthURL    string
	lastChecked  time.Time
	logger       Logger
}

// NewCircuitBreaker creates a new CircuitBreaker instance based on the provided config.
func NewCircuitBreaker(config CircuitBreakerConfig, logger Logger) *CircuitBreaker {
	if !config.Enabled {
		return nil
	}

	cb := &CircuitBreaker{
		state:     ClosedState,
		maxRetry:  config.MaxRetry,
		threshold: config.Threshold,
		timeout:   config.Timeout,
		interval:  config.Interval,
		healthURL: config.HealthURL,
		logger:    logger,
	}

	// Perform asynchronous health checks
	go cb.startHealthChecks()

	return cb
}

// ExecuteWithCircuitBreaker executes the given function with circuit breaker protection.
func (cb *CircuitBreaker) ExecuteWithCircuitBreaker(ctx context.Context, f func(ctx context.Context) (*http.Response,
	error)) (*http.Response, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == OpenState {
		if time.Since(cb.lastChecked) > cb.timeout {
			// Check health before potentially closing the circuit
			if cb.healthCheck() {
				cb.resetCircuit()
				return nil, nil
			}
		}

		return nil, ErrCircuitOpen
	}

	if cb.failureCount > cb.threshold {
		cb.openCircuit()
		return nil, ErrCircuitOpen
	}

	result, err := f(ctx)

	if err != nil {
		cb.handleFailure()
	} else {
		cb.resetFailureCount()
	}

	return result, err
}

// IsOpen returns true if the circuit breaker is in the open state.
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	return cb.state == OpenState
}

// FailureCount returns the current failure count.
func (cb *CircuitBreaker) FailureCount() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	return cb.failureCount
}

// healthCheck performs the health check for the circuit breaker.
func (cb *CircuitBreaker) healthCheck() bool {
	if cb.healthURL == "" {
		cb.logger.Log("Circuit breaker: Missing health check URL")
		return false
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, cb.healthURL, http.NoBody)
	if err != nil {
		return false
	}

	resp, err := http.DefaultClient.Do(req) // Use http.DefaultClient with context
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// startHealthChecks initiates periodic health checks.
func (cb *CircuitBreaker) startHealthChecks() {
	ticker := time.NewTicker(cb.interval)

	for range ticker.C {
		if cb.IsOpen() {
			go func() {
				if cb.healthCheck() {
					cb.resetCircuit()
				}
			}()
		}
	}
}

// openCircuit transitions the circuit breaker to the open state.
func (cb *CircuitBreaker) openCircuit() {
	cb.state = OpenState
	cb.lastChecked = time.Now()
}

// resetCircuit transitions the circuit breaker to the closed state.
func (cb *CircuitBreaker) resetCircuit() {
	cb.state = ClosedState
	cb.failureCount = 0
}

// handleFailure increments the failure count and opens the circuit if the threshold is reached.
func (cb *CircuitBreaker) handleFailure() {
	cb.failureCount++
	if cb.failureCount > cb.threshold {
		cb.openCircuit()
	}
}

// resetFailureCount resets the failure count to zero.
func (cb *CircuitBreaker) resetFailureCount() {
	cb.failureCount = 0
}

func (cbc *CircuitBreakerConfig) apply(h *httpService, logger Logger) {
	cb := NewCircuitBreaker(*cbc, logger)
	h.CircuitBreaker = cb
}
