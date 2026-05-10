package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xssnick/tonutils-go/adnl"
	"github.com/xssnick/tonutils-go/adnl/dht"
	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-storage-provider/pkg/transport"
)

var logLevels = map[uint8]slog.Level{
	0: slog.LevelDebug,
	1: slog.LevelInfo,
	2: slog.LevelWarn,
	3: slog.LevelError,
}

type System struct {
	ADNLPort string             `env:"SYSTEM_ADNL_PORT" envDefault:"16167"`
	Key      ed25519.PrivateKey `env:"SYSTEM_KEY" required:"false"`
	LogLevel uint8              `env:"SYSTEM_LOG_LEVEL" envDefault:"1"`
}

type TON struct {
	ConfigURL     string `env:"TON_CONFIG_URL" envDefault:"https://ton-blockchain.github.io/global.config.json"`
	MasterAddress string `env:"MASTER_ADDRESS" envDefault:"UQB3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d0x0"`
	BatchSize     uint32 `env:"BATCH_SIZE" envDefault:"100"`
}

type Postgress struct {
	Host     string `env:"DB_HOST"`
	Port     string `env:"DB_PORT"`
	User     string `env:"DB_USER"`
	Password string `env:"DB_PASSWORD"`
	Name     string `env:"DB_NAME"`
}

type Config struct {
	System System
	TON    TON
	DB     Postgress
}

func loadConfig(postgres bool) *Config {
	cfg := &Config{}
	if err := env.Parse(&cfg.System); err != nil {
		log.Fatalf("Failed to parse system config: %v", err)
	}
	if err := env.Parse(&cfg.TON); err != nil {
		log.Fatalf("Failed to parse TON config: %v", err)
	}
	if postgres {
		if err := env.Parse(&cfg.DB); err != nil {
			log.Fatalf("Failed to parse db config: %v", err)
		}
		if err := validateDBConfig(cfg.DB); err != nil {
			log.Fatalf("Database config: %v\n\nSet DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME for --storage=postgres.", err)
		}
	}

	if cfg.System.Key == nil {
		_, priv, _ := ed25519.GenerateKey(nil)
		key := priv.Seed()
		cfg.System.Key = ed25519.NewKeyFromSeed(key)
	}

	return cfg
}

func validateDBConfig(db Postgress) error {
	var missing []string
	if strings.TrimSpace(db.Host) == "" {
		missing = append(missing, "DB_HOST")
	}
	if strings.TrimSpace(db.Port) == "" {
		missing = append(missing, "DB_PORT")
	}
	if strings.TrimSpace(db.User) == "" {
		missing = append(missing, "DB_USER")
	}
	if strings.TrimSpace(db.Password) == "" {
		missing = append(missing, "DB_PASSWORD")
	}
	if strings.TrimSpace(db.Name) == "" {
		missing = append(missing, "DB_NAME")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing or empty: %s", strings.Join(missing, ", "))
	}
	return nil
}

func connectPostgres(ctx context.Context, config *Config, logger *slog.Logger) (connPool *pgxpool.Pool, err error) {
	cfg, err := newPostgresConfig(config, logger)
	if err != nil {
		return nil, err
	}

	connPool, err = pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new Postgres connection pool: %w", err)
	}

	connection, err := connPool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire a connection from the Postgres pool: %w", err)
	}
	defer connection.Release()

	if err := connection.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping the Postgres database: %w", err)
	}

	return connPool, nil
}

func newPostgresConfig(config *Config, logger *slog.Logger) (dbConfig *pgxpool.Config, err error) {
	const (
		defaultMaxConns        = int32(12)
		defaultMinConns        = int32(3)
		defaultMaxConnLifetime = time.Hour
		defaultMaxConnIdleTime = 30 * time.Minute
		defaultHealthCheck     = time.Minute
		defaultConnectTimeout  = 5 * time.Second
		databaseURLPattern     = "postgres://%s:%s@%s:%s/%s"
	)

	user := url.QueryEscape(config.DB.User)
	password := url.QueryEscape(config.DB.Password)
	pgURL := fmt.Sprintf(databaseURLPattern, user, password, config.DB.Host, config.DB.Port, config.DB.Name)

	dbConfig, err = pgxpool.ParseConfig(pgURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Postgres connection string: %w", err)
	}

	dbConfig.MaxConns = defaultMaxConns
	dbConfig.MinConns = defaultMinConns
	dbConfig.MaxConnLifetime = defaultMaxConnLifetime
	dbConfig.MaxConnIdleTime = defaultMaxConnIdleTime
	dbConfig.HealthCheckPeriod = defaultHealthCheck
	dbConfig.ConnConfig.ConnectTimeout = defaultConnectTimeout
	dbConfig.BeforeAcquire = func(ctx context.Context, c *pgx.Conn) bool { return true }
	dbConfig.AfterRelease = func(c *pgx.Conn) bool { return true }
	dbConfig.BeforeClose = func(c *pgx.Conn) {
		logger.Info("closed the connection pool to the database")
	}

	return dbConfig, nil
}

func newProviderClient(ctx context.Context, configURL, adnlPort string, privateKey ed25519.PrivateKey) (dc *dht.Client, tc *transport.Client, err error) {
	lsCfg, err := liteclient.GetConfigFromUrl(ctx, configURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get liteclient config: %w", err)
	}

	_, dhtAdnlKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate DHT ADNL key: %w", err)
	}

	dl, err := adnl.DefaultListener("0.0.0.0:" + adnlPort)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create default listener: %w", err)
	}

	netMgr := adnl.NewMultiNetReader(dl)

	dhtGate := adnl.NewGatewayWithNetManager(dhtAdnlKey, netMgr)
	if err = dhtGate.StartClient(); err != nil {
		return nil, nil, fmt.Errorf("failed to start DHT gateway: %w", err)
	}

	dc, err = dht.NewClientFromConfig(dhtGate, lsCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create DHT client: %w", err)
	}

	gateProvider := adnl.NewGatewayWithNetManager(privateKey, netMgr)
	if err = gateProvider.StartClient(); err != nil {
		return nil, nil, fmt.Errorf("failed to start ADNL gateway for provider: %w", err)
	}

	tc = transport.NewClient(gateProvider, dc)
	return dc, tc, nil
}
