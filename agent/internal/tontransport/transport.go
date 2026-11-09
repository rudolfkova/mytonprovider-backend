package tontransport

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/xssnick/tonutils-go/adnl"
	"github.com/xssnick/tonutils-go/adnl/dht"
	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-storage-provider/pkg/transport"
)

// ProviderTransport holds DHT + ADNL resources used by transport.Client (GetStorageRates, etc.).
// It must not share an ADNL UDP port with another process (e.g. coordinator) on the same host.
type ProviderTransport struct {
	dhtGateway      *adnl.Gateway
	providerGateway *adnl.Gateway
	netMgr          adnl.NetManager
	dhtClient       *dht.Client
	peers           *peerSessions[*transport.Client]

	startOnce sync.Once
	closeOnce sync.Once
	startErr  error

	mu     sync.Mutex
	closed bool
	logger *slog.Logger

	tonConfigURL string
	adnlPort     string
	privateKey   ed25519.PrivateKey
}

// NewProviderTransport configures transport; call Start before using Client.
func NewProviderTransport(tonConfigURL, adnlPort string, privateKey ed25519.PrivateKey, logger *slog.Logger) *ProviderTransport {
	if logger == nil {
		logger = slog.Default()
	}
	p := &ProviderTransport{
		tonConfigURL: tonConfigURL,
		adnlPort:     adnlPort,
		privateKey:   privateKey,
		logger:       logger,
	}
	p.peers = newPeerSessions(providerPeerResetCooldown, p.recreateProviderClient, func() {
		p.logger.Info("reset provider ADNL gateway")
	})
	return p
}

// Start loads TON lite config, binds UDP, and wires DHT + provider transport client.
func (p *ProviderTransport) Start(ctx context.Context) error {
	p.startOnce.Do(func() {
		p.startErr = p.startLocked(ctx)
	})
	return p.startErr
}

func (p *ProviderTransport) startLocked(ctx context.Context) error {
	lsCfg, err := liteclient.GetConfigFromUrl(ctx, p.tonConfigURL)
	if err != nil {
		return fmt.Errorf("get liteclient config: %w", err)
	}

	_, dhtAdnlKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		return fmt.Errorf("generate DHT ADNL key: %w", err)
	}

	dl, err := adnl.DefaultListener("0.0.0.0:" + p.adnlPort)
	if err != nil {
		return fmt.Errorf("create ADNL listener: %w", err)
	}

	netMgr := adnl.NewMultiNetReader(dl)

	dhtGate := adnl.NewGatewayWithNetManager(dhtAdnlKey, netMgr)
	if err = dhtGate.StartClient(); err != nil {
		return fmt.Errorf("start DHT gateway: %w", err)
	}

	dc, err := dht.NewClientFromConfig(dhtGate, lsCfg)
	if err != nil {
		_ = dhtGate.Close()
		return fmt.Errorf("create DHT client: %w", err)
	}

	gateProvider := adnl.NewGatewayWithNetManager(p.privateKey, netMgr)
	if err = gateProvider.StartClient(); err != nil {
		_ = dhtGate.Close()
		return fmt.Errorf("start provider ADNL gateway: %w", err)
	}

	p.mu.Lock()
	p.netMgr = netMgr
	p.dhtClient = dc
	p.dhtGateway = dhtGate
	p.providerGateway = gateProvider
	p.mu.Unlock()

	p.peers.setInitial(transport.NewClient(gateProvider, dc))
	return nil
}

// Client returns the tonutils transport client after Start succeeds.
func (p *ProviderTransport) Client() *transport.Client {
	client, _ := p.peers.snapshot()
	return client
}

// Call runs fn against the current provider client.
// A transport error with a live parent context resets cached ADNL peers (at most once per cooldown)
// and retries fn once with a new perQuery timeout.
func (p *ProviderTransport) Call(ctx context.Context, perQuery time.Duration, fn func(context.Context, *transport.Client) error, onRetry func(error)) error {
	return callWithPeerReset(ctx, perQuery, p.peers.snapshot, p.peers.resetIfDue, fn, onRetry)
}

// recreateProviderClient drops the provider gateway so RegisterClient cannot hand back a stuck peer.
// The DHT gateway stays up, which keeps the shared UDP listener open.
func (p *ProviderTransport) recreateProviderClient() (*transport.Client, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, fmt.Errorf("provider transport is closed")
	}
	old := p.providerGateway
	netMgr := p.netMgr
	dhtClient := p.dhtClient
	key := p.privateKey
	p.providerGateway = nil
	p.mu.Unlock()

	if netMgr == nil || dhtClient == nil {
		return nil, fmt.Errorf("provider transport is not started")
	}
	if old != nil {
		_ = old.Close()
	}

	gate := adnl.NewGatewayWithNetManager(key, netMgr)
	if err := gate.StartClient(); err != nil {
		return nil, fmt.Errorf("restart provider ADNL gateway: %w", err)
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		_ = gate.Close()
		return nil, fmt.Errorf("provider transport is closed")
	}
	p.providerGateway = gate
	p.mu.Unlock()

	return transport.NewClient(gate, dhtClient), nil
}

// Close stops gateways and releases the UDP listener.
func (p *ProviderTransport) Close() error {
	var firstErr error
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		providerGateway := p.providerGateway
		dhtGateway := p.dhtGateway
		p.providerGateway = nil
		p.dhtGateway = nil
		p.mu.Unlock()

		if providerGateway != nil {
			if err := providerGateway.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		if dhtGateway != nil {
			if err := dhtGateway.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	})
	return firstErr
}
