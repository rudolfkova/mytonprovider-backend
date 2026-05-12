package config

import (
	"fmt"
	"os"
	"strconv"
)

const (
	defaultListenAddr             = ":8443"
	defaultAgentID                = "agent"
	defaultLocation               = "unknown"
	defaultMaxConcurrentProviders = 40
)

type Config struct {
	ListenAddr             string
	AgentID                string
	Location               string
	AuthToken              string
	TLSCertFile            string
	TLSKeyFile             string
	MaxConcurrentProviders int
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddr:             envOrDefault("AGENT_LISTEN_ADDR", defaultListenAddr),
		AgentID:                envOrDefault("AGENT_ID", defaultAgentID),
		Location:               envOrDefault("AGENT_LOCATION", defaultLocation),
		AuthToken:              os.Getenv("AGENT_AUTH_TOKEN"),
		TLSCertFile:            os.Getenv("AGENT_TLS_CERT_FILE"),
		TLSKeyFile:             os.Getenv("AGENT_TLS_KEY_FILE"),
		MaxConcurrentProviders: defaultMaxConcurrentProviders,
	}

	if s := os.Getenv("AGENT_MAX_CONCURRENT_PROVIDERS"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 {
			return Config{}, fmt.Errorf("invalid AGENT_MAX_CONCURRENT_PROVIDERS: %q", s)
		}
		cfg.MaxConcurrentProviders = v
	}

	if cfg.AuthToken == "" {
		return Config{}, fmt.Errorf("AGENT_AUTH_TOKEN is required")
	}
	if cfg.TLSCertFile == "" {
		return Config{}, fmt.Errorf("AGENT_TLS_CERT_FILE is required")
	}
	if cfg.TLSKeyFile == "" {
		return Config{}, fmt.Errorf("AGENT_TLS_KEY_FILE is required")
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
