package tontransport

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"sync"

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
	client          *transport.Client

	startOnce sync.Once
	closeOnce sync.Once
	startErr  error

	tonConfigURL string
	adnlPort     string
	privateKey   ed25519.PrivateKey
}

// NewProviderTransport configures transport; call Start before using Client.
func NewProviderTransport(tonConfigURL, adnlPort string, privateKey ed25519.PrivateKey) *ProviderTransport {
	return &ProviderTransport{
		tonConfigURL: tonConfigURL,
		adnlPort:     adnlPort,
		privateKey:   privateKey,
	}
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
	p.dhtGateway = dhtGate

	dc, err := dht.NewClientFromConfig(dhtGate, lsCfg)
	if err != nil {
		_ = dhtGate.Close()
		p.dhtGateway = nil
		return fmt.Errorf("create DHT client: %w", err)
	}

	gateProvider := adnl.NewGatewayWithNetManager(p.privateKey, netMgr)
	if err = gateProvider.StartClient(); err != nil {
		_ = dhtGate.Close()
		p.dhtGateway = nil
		return fmt.Errorf("start provider ADNL gateway: %w", err)
	}
	p.providerGateway = gateProvider

	p.client = transport.NewClient(gateProvider, dc)
	return nil
}

// Client returns the tonutils transport client after Start succeeds.
func (p *ProviderTransport) Client() *transport.Client {
	return p.client
}

// Close stops gateways and releases the UDP listener.
func (p *ProviderTransport) Close() error {
	var firstErr error
	p.closeOnce.Do(func() {
		if p.providerGateway != nil {
			if err := p.providerGateway.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
			p.providerGateway = nil
		}
		if p.dhtGateway != nil {
			if err := p.dhtGateway.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
			p.dhtGateway = nil
		}
		p.client = nil
	})
	return firstErr
}
