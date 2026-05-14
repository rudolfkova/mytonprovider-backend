package agentrpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	providerchecksv1 "mytonprovider-contracts/gen/go/providerchecks/v1"
)

type Config struct {
	Endpoints      []string
	AuthToken      string
	CACertFile     string
	RequestTimeout time.Duration
}

type RunChecksResult struct {
	Endpoint string
	Response *providerchecksv1.RunChecksResponse
}

type RunStorageRatesResult struct {
	Endpoint string
	Response *providerchecksv1.RunStorageRatesResponse
}

type AgentCallError struct {
	Endpoint string
	Err      error
}

type Client struct {
	agents         []agentClient
	authToken      string
	requestTimeout time.Duration
}

type agentClient struct {
	endpoint string
	conn     *grpc.ClientConn
	client   providerchecksv1.ProviderChecksServiceClient
}

func New(cfg Config, logger *slog.Logger) (*Client, error) {
	if len(cfg.Endpoints) == 0 {
		return &Client{}, nil
	}
	if strings.TrimSpace(cfg.AuthToken) == "" {
		return nil, fmt.Errorf("agent auth token is required when AGENT_ENDPOINTS is set")
	}
	if strings.TrimSpace(cfg.CACertFile) == "" {
		return nil, fmt.Errorf("agent CA cert path is required when AGENT_ENDPOINTS is set")
	}

	creds, err := loadTLSCredentials(cfg.CACertFile)
	if err != nil {
		return nil, err
	}

	requestTimeout := cfg.RequestTimeout
	if requestTimeout <= 0 {
		requestTimeout = 30 * time.Second
	}

	clients := make([]agentClient, 0, len(cfg.Endpoints))
	for _, rawEndpoint := range cfg.Endpoints {
		endpoint := strings.TrimSpace(rawEndpoint)
		if endpoint == "" {
			continue
		}

		conn, err := grpc.Dial(
			endpoint,
			grpc.WithTransportCredentials(creds),
		)
		if err != nil {
			if logger != nil {
				logger.Error("failed to create gRPC client connection to agent", "endpoint", endpoint, "error", err)
			}
			continue
		}

		clients = append(clients, agentClient{
			endpoint: endpoint,
			conn:     conn,
			client:   providerchecksv1.NewProviderChecksServiceClient(conn),
		})
	}

	return &Client{
		agents:         clients,
		authToken:      strings.TrimSpace(cfg.AuthToken),
		requestTimeout: requestTimeout,
	}, nil
}

func (c *Client) Close() error {
	var firstErr error
	for _, a := range c.agents {
		if a.conn == nil {
			continue
		}
		if err := a.conn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (c *Client) AgentCount() int {
	if c == nil {
		return 0
	}
	return len(c.agents)
}

func (c *Client) RunChecksAll(ctx context.Context, req *providerchecksv1.RunChecksRequest) ([]RunChecksResult, []AgentCallError) {
	if c == nil || len(c.agents) == 0 {
		return nil, nil
	}

	results := make([]RunChecksResult, 0, len(c.agents))
	errs := make([]AgentCallError, 0)

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, agent := range c.agents {
		wg.Add(1)
		go func(agent agentClient) {
			defer wg.Done()

			callCtx := ctx
			cancel := func() {}
			if c.requestTimeout > 0 {
				callCtx, cancel = context.WithTimeout(ctx, c.requestTimeout)
			}
			defer cancel()
			callCtx = metadata.AppendToOutgoingContext(callCtx, "authorization", "Bearer "+c.authToken)

			resp, err := agent.client.RunChecks(callCtx, req)
			if err != nil {
				mu.Lock()
				errs = append(errs, AgentCallError{
					Endpoint: agent.endpoint,
					Err:      err,
				})
				mu.Unlock()
				return
			}

			mu.Lock()
			results = append(results, RunChecksResult{
				Endpoint: agent.endpoint,
				Response: resp,
			})
			mu.Unlock()
		}(agent)
	}
	wg.Wait()

	return results, errs
}

func (c *Client) RunStorageRatesAll(ctx context.Context, req *providerchecksv1.RunStorageRatesRequest) ([]RunStorageRatesResult, []AgentCallError) {
	if c == nil || len(c.agents) == 0 {
		return nil, nil
	}

	results := make([]RunStorageRatesResult, 0, len(c.agents))
	errs := make([]AgentCallError, 0)

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, agent := range c.agents {
		wg.Add(1)
		go func(agent agentClient) {
			defer wg.Done()

			callCtx := ctx
			cancel := func() {}
			if c.requestTimeout > 0 {
				callCtx, cancel = context.WithTimeout(ctx, c.requestTimeout)
			}
			defer cancel()
			callCtx = metadata.AppendToOutgoingContext(callCtx, "authorization", "Bearer "+c.authToken)

			resp, err := agent.client.RunStorageRates(callCtx, req)
			if err != nil {
				mu.Lock()
				errs = append(errs, AgentCallError{
					Endpoint: agent.endpoint,
					Err:      err,
				})
				mu.Unlock()
				return
			}

			mu.Lock()
			results = append(results, RunStorageRatesResult{
				Endpoint: agent.endpoint,
				Response: resp,
			})
			mu.Unlock()
		}(agent)
	}
	wg.Wait()

	return results, errs
}

func ParseEndpointsCSV(csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func loadTLSCredentials(caCertFile string) (credentials.TransportCredentials, error) {
	caPEM, err := os.ReadFile(caCertFile)
	if err != nil {
		return nil, fmt.Errorf("read CA cert file: %w", err)
	}

	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(caPEM); !ok {
		return nil, fmt.Errorf("append CA cert to pool: invalid PEM")
	}

	return credentials.NewTLS(&tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    pool,
	}), nil
}
