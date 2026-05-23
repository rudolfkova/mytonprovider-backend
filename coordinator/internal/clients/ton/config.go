package tonclient

import (
	"context"
	"fmt"
	"strings"

	"github.com/xssnick/tonutils-go/liteclient"
)

// LoadGlobalConfig loads TON global config from an HTTPS URL or a local JSON file path.
// Local paths may use a file:// prefix or be absolute/relative filesystem paths.
func LoadGlobalConfig(ctx context.Context, configURL string) (*liteclient.GlobalConfig, error) {
	configURL = strings.TrimSpace(configURL)
	if configURL == "" {
		return nil, fmt.Errorf("TON config URL is empty")
	}

	if path, ok := localConfigPath(configURL); ok {
		return liteclient.GetConfigFromFile(path)
	}

	return liteclient.GetConfigFromUrl(ctx, configURL)
}

func localConfigPath(configURL string) (string, bool) {
	if strings.HasPrefix(configURL, "file://") {
		return strings.TrimPrefix(configURL, "file://"), true
	}
	lower := strings.ToLower(configURL)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return "", false
	}
	// Reject mistaken scheme URLs; only http(s) and file paths are supported.
	if strings.Contains(configURL, "://") {
		return "", false
	}
	return configURL, true
}

// AddConnectionsFromConfigURL connects the pool using config from URL or file (see LoadGlobalConfig).
func AddConnectionsFromConfigURL(ctx context.Context, pool *liteclient.ConnectionPool, configURL string) error {
	cfg, err := LoadGlobalConfig(ctx, configURL)
	if err != nil {
		return err
	}
	return pool.AddConnectionsFromConfig(ctx, cfg)
}
