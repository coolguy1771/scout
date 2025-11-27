package opensearch

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	opensearch "github.com/opensearch-project/opensearch-go/v4"
	opensearchapi "github.com/opensearch-project/opensearch-go/v4/opensearchapi"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/config"
)

// Client wraps the OpenSearch client with connection handling
type Client struct {
	client  *opensearch.Client
	config  *config.SearchConfig
	logger  *zap.Logger
	enabled bool
}

// NewClient creates a new OpenSearch client
func NewClient(cfg *config.SearchConfig, logger *zap.Logger) (*Client, error) {
	if !cfg.Enabled {
		logger.Info("OpenSearch is disabled, search will use PostGIS fallback")
		return &Client{
			config:  cfg,
			logger:  logger,
			enabled: false,
		}, nil
	}

	addresses := []string{
		fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port),
	}

	clientConfig := opensearch.Config{
		Addresses: addresses,
	}

	// Add authentication if provided
	if cfg.User != "" && cfg.Password != "" {
		clientConfig.Username = cfg.User
		clientConfig.Password = cfg.Password
	}

	client, err := opensearch.NewClient(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenSearch client: %w", err)
	}

	// Test connection
	infoReq := opensearchapi.InfoReq{}
	var infoRes opensearchapi.InfoResp
	_, err = client.Do(context.Background(), &infoReq, &infoRes)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to OpenSearch: %w", err)
	}

	if infoRes.Inspect().Response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenSearch health check failed with status %d", infoRes.Inspect().Response.StatusCode)
	}

	logger.Info("OpenSearch client connected successfully",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port))

	return &Client{
		client:  client,
		config:  cfg,
		logger:  logger,
		enabled: true,
	}, nil
}

// IsEnabled returns whether OpenSearch is enabled and connected
func (c *Client) IsEnabled() bool {
	return c.enabled && c.client != nil
}

// GetClient returns the underlying OpenSearch client (may be nil if disabled)
func (c *Client) GetClient() *opensearch.Client {
	return c.client
}

// HealthCheck checks the health of the OpenSearch cluster
func (c *Client) HealthCheck(ctx context.Context) error {
	if !c.IsEnabled() {
		return fmt.Errorf("OpenSearch is not enabled")
	}

	healthReq := opensearchapi.ClusterHealthReq{
		Params: opensearchapi.ClusterHealthParams{
			Level: "cluster",
		},
	}
	var healthRes opensearchapi.ClusterHealthResp

	_, err := c.client.Do(ctx, &healthReq, &healthRes)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	if healthRes.Inspect().Response.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", healthRes.Inspect().Response.StatusCode)
	}

	return nil
}

// Ping checks if OpenSearch is reachable
func (c *Client) Ping(ctx context.Context) error {
	if !c.IsEnabled() {
		return fmt.Errorf("OpenSearch is not enabled")
	}

	// Ping is just a HEAD request, we can use Info as a simple check
	infoReq := opensearchapi.InfoReq{}
	var infoRes opensearchapi.InfoResp
	_, err := c.client.Do(ctx, &infoReq, &infoRes)
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	if infoRes.Inspect().Response.StatusCode != http.StatusOK {
		return fmt.Errorf("ping returned status %d", infoRes.Inspect().Response.StatusCode)
	}

	return nil
}

// BuildURL constructs a URL for OpenSearch requests
func (c *Client) BuildURL(path string) string {
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}
	return fmt.Sprintf("http://%s:%d/%s", c.config.Host, c.config.Port, path)
}
