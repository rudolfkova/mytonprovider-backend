package config

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultListenAddr             = ":8443"
	defaultAgentID                = "agent"
	defaultLocation               = "unknown"
	defaultMaxConcurrentProviders = 40
	defaultTonConfigURL           = "https://ton-blockchain.github.io/global.config.json"
	defaultAdnlPort               = "16167"
)

type Config struct {
	ListenAddr             string
	AgentID                string
	Location               string
	AuthToken              string
	TLSCertFile            string
	TLSKeyFile             string
	MaxConcurrentProviders int
	// TonConfigURL is the TON global config URL for liteclient/DHT (RunStorageRates).
	TonConfigURL string
	// AdnlPort is the UDP port for ADNL (must not conflict with coordinator on the same host).
	AdnlPort string
	// AdnlPrivateKey is used for the provider transport gateway (GetStorageRates).
	AdnlPrivateKey ed25519.PrivateKey
	// MaxConcurrentRates limits parallel GetStorageRates calls; 0 means use MaxConcurrentProviders.
	MaxConcurrentRates int
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
		TonConfigURL:           envOrDefault("AGENT_TON_CONFIG_URL", defaultTonConfigURL),
		AdnlPort:               envOrDefault("AGENT_ADNL_PORT", defaultAdnlPort),
	}

	if s := os.Getenv("AGENT_MAX_CONCURRENT_PROVIDERS"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 {
			return Config{}, fmt.Errorf("invalid AGENT_MAX_CONCURRENT_PROVIDERS: %q", s)
		}
		cfg.MaxConcurrentProviders = v
	}

	if s := os.Getenv("AGENT_MAX_CONCURRENT_RATES"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 {
			return Config{}, fmt.Errorf("invalid AGENT_MAX_CONCURRENT_RATES: %q", s)
		}
		cfg.MaxConcurrentRates = v
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

	seedHex := strings.TrimSpace(os.Getenv("AGENT_ADNL_KEY"))
	if seedHex != "" {
		raw, err := hex.DecodeString(seedHex)
		if err != nil {
			return Config{}, fmt.Errorf("decode AGENT_ADNL_KEY: %w", err)
		}
		if len(raw) != ed25519.SeedSize {
			return Config{}, fmt.Errorf("AGENT_ADNL_KEY must be %d hex chars (Ed25519 seed)", ed25519.SeedSize*2)
		}
		cfg.AdnlPrivateKey = ed25519.NewKeyFromSeed(raw)
	} else {
		_, priv, err := ed25519.GenerateKey(nil)
		if err != nil {
			return Config{}, fmt.Errorf("generate ADNL key: %w", err)
		}
		cfg.AdnlPrivateKey = priv
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
