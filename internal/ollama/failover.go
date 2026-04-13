package ollama

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type EndpointPoolClient struct {
	endpoints           []*poolEndpoint
	healthCheckInterval time.Duration
	logger              *slog.Logger
}

type poolEndpoint struct {
	client *Client

	mu        sync.RWMutex
	healthy   bool
	lastCheck time.Time
	lastError string
}

func NewEndpointPoolClient(clients []*Client, healthCheckInterval time.Duration, logger *slog.Logger) *EndpointPoolClient {
	filtered := make([]*poolEndpoint, 0, len(clients))
	for _, client := range clients {
		if client == nil {
			continue
		}
		filtered = append(filtered, &poolEndpoint{
			client:  client,
			healthy: true,
		})
	}

	if healthCheckInterval <= 0 {
		healthCheckInterval = 30 * time.Minute
	}

	return &EndpointPoolClient{
		endpoints:           filtered,
		healthCheckInterval: healthCheckInterval,
		logger:              logger,
	}
}

func (c *EndpointPoolClient) Chat(ctx context.Context, messages []Message) (string, error) {
	if c == nil || len(c.endpoints) == 0 {
		return "", fmt.Errorf("ollama endpoint pool is empty")
	}

	order := c.requestOrder()
	var errs []string

	for _, idx := range order {
		endpoint := c.endpoints[idx]
		reply, err := endpoint.client.Chat(ctx, messages)
		if err == nil {
			endpoint.markHealth(true, "")
			return reply, nil
		}

		if !isFailoverEligible(err) {
			return "", err
		}

		endpoint.markHealth(false, err.Error())
		errs = append(errs, fmt.Sprintf("endpoint[%d]=%s", idx, err.Error()))
		if c.logger != nil {
			c.logger.Warn("ollama endpoint failed, trying next endpoint", "endpoint_index", idx, "base_url", endpoint.client.baseURL, "error", err)
		}
	}

	return "", fmt.Errorf("all ollama endpoints failed: %s", strings.Join(errs, "; "))
}

func (c *EndpointPoolClient) Run(ctx context.Context) error {
	if c == nil || len(c.endpoints) == 0 {
		return nil
	}

	c.probeEndpoints(ctx)

	ticker := time.NewTicker(c.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			c.refreshUnhealthyEndpoints(ctx)
		}
	}
}

func (c *EndpointPoolClient) probeEndpoints(ctx context.Context) {
	activeIndex := -1
	for idx, endpoint := range c.endpoints {
		checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := endpoint.client.CheckHealth(checkCtx)
		cancel()

		if err != nil {
			endpoint.markHealth(false, err.Error())
			if c.logger != nil {
				c.logger.Warn("ollama endpoint probe failed", "endpoint_index", idx, "base_url", endpoint.client.baseURL, "error", err)
			}
			continue
		}

		endpoint.markHealth(true, "")
		if c.logger != nil {
			c.logger.Info("ollama endpoint connected", "endpoint_index", idx, "base_url", endpoint.client.baseURL, "checked_at", time.Now().Format(time.RFC3339))
		}
		if activeIndex == -1 {
			activeIndex = idx
		}
	}

	if c.logger != nil && activeIndex >= 0 {
		c.logger.Info("ollama active endpoint selected", "endpoint_index", activeIndex, "base_url", c.endpoints[activeIndex].client.baseURL, "selected_at", time.Now().Format(time.RFC3339))
	}
}

func (c *EndpointPoolClient) requestOrder() []int {
	healthy := make([]int, 0, len(c.endpoints))
	unhealthy := make([]int, 0, len(c.endpoints))

	for idx, endpoint := range c.endpoints {
		if endpoint.isHealthy() {
			healthy = append(healthy, idx)
			continue
		}
		unhealthy = append(unhealthy, idx)
	}

	if len(healthy) > 0 {
		return healthy
	}
	return unhealthy
}

func (c *EndpointPoolClient) refreshUnhealthyEndpoints(ctx context.Context) {
	for idx, endpoint := range c.endpoints {
		if endpoint.isHealthy() {
			continue
		}

		wasHealthy := endpoint.isHealthy()
		checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := endpoint.client.CheckHealth(checkCtx)
		cancel()

		if err != nil {
			endpoint.markHealth(false, err.Error())
			continue
		}

		endpoint.markHealth(true, "")
		if c.logger != nil && !wasHealthy {
			c.logger.Info("ollama endpoint recovered", "endpoint_index", idx, "base_url", endpoint.client.baseURL, "recovered_at", time.Now().Format(time.RFC3339))
		}
	}
}

func (e *poolEndpoint) isHealthy() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.healthy
}

func (e *poolEndpoint) markHealth(healthy bool, errText string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.healthy = healthy
	e.lastCheck = time.Now()
	e.lastError = strings.TrimSpace(errText)
}

func isFailoverEligible(err error) bool {
	if err == nil {
		return false
	}

	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(lower, "timeout"):
		return true
	case strings.Contains(lower, "deadline exceeded"):
		return true
	case strings.Contains(lower, "connection refused"):
		return true
	case strings.Contains(lower, "connection reset by peer"):
		return true
	case strings.Contains(lower, "no such host"):
		return true
	case strings.Contains(lower, "network is unreachable"):
		return true
	case strings.Contains(lower, "bad gateway"):
		return true
	case strings.Contains(lower, "ollama returned 5"):
		return true
	case strings.Contains(lower, "ollama health returned 5"):
		return true
	default:
		return false
	}
}
