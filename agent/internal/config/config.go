package config

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultListenAddr             = ":8443"
	defaultAgentID                = "agent"
	defaultLocation               = "unknown"
	defaultMaxConcurrentProviders = 40
	defaultMaxConcurrentBags      = 4
	defaultPieceMinBPS            = 312500
	defaultPieceTimeoutCap        = 15 * time.Second
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
	// MaxConcurrentBags is how many contracts of one provider may be checked at once.
	MaxConcurrentBags int
	// PieceMinBPS is the assumed floor throughput used to size the GetPiece timeout.
	PieceMinBPS uint64
	// PieceTimeoutCap is the upper bound of a GetPiece timeout.
	PieceTimeoutCap time.Duration
	// TonConfigURL is the TON global config URL for liteclient/DHT (RunStorageRates).
	TonConfigURL string
	// AdnlPort is the UDP port for ADNL (must not conflict with coordinator on the same host).
	AdnlPort string
	// AdnlPrivateKey is used for the provider transport gateway (GetStorageRates).
	AdnlPrivateKey ed25519.PrivateKey
	// MaxConcurrentRates limits parallel GetStorageRates calls; 0 means use MaxConcurrentProviders.
	MaxConcurrentRates int
	// MetricsListenAddr is the TCP address for Prometheus /metrics (HTTP). Empty disables metrics.
	MetricsListenAddr string
	// LokiURL is the base URL for Loki HTTP push (e.g. http://127.0.0.1:3100). Empty disables RunChecks push.
	LokiURL string
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
		MaxConcurrentBags:      defaultMaxConcurrentBags,
		PieceMinBPS:            defaultPieceMinBPS,
		PieceTimeoutCap:        defaultPieceTimeoutCap,
		TonConfigURL:           envOrDefault("AGENT_TON_CONFIG_URL", defaultTonConfigURL),
		AdnlPort:               envOrDefault("AGENT_ADNL_PORT", defaultAdnlPort),
		MetricsListenAddr:      strings.TrimSpace(os.Getenv("AGENT_METRICS_LISTEN_ADDR")),
		LokiURL:                strings.TrimSpace(strings.TrimRight(os.Getenv("AGENT_LOKI_URL"), "/")),
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

	if s := os.Getenv("AGENT_MAX_CONCURRENT_BAGS"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 {
			return Config{}, fmt.Errorf("invalid AGENT_MAX_CONCURRENT_BAGS: %q", s)
		}
		cfg.MaxConcurrentBags = v
	}

	if s := os.Getenv("AGENT_PIECE_MIN_BPS"); s != "" {
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil || v == 0 {
			return Config{}, fmt.Errorf("invalid AGENT_PIECE_MIN_BPS: %q", s)
		}
		cfg.PieceMinBPS = v
	}

	if s := os.Getenv("AGENT_PIECE_TIMEOUT_CAP_MS"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 {
			return Config{}, fmt.Errorf("invalid AGENT_PIECE_TIMEOUT_CAP_MS: %q", s)
		}
		cfg.PieceTimeoutCap = time.Duration(v) * time.Millisecond
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
