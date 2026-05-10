package checker

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"log/slog"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/xssnick/tonutils-go/adnl"
	"github.com/xssnick/tonutils-go/adnl/keys"
	"github.com/xssnick/tonutils-go/adnl/overlay"
	"github.com/xssnick/tonutils-go/adnl/rldp"
	"github.com/xssnick/tonutils-go/tl"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/tvm/cell"
	"github.com/xssnick/tonutils-storage/storage"

	providerchecksv1 "github.com/grach/mytonprovider-contracts/gen/go/providerchecks/v1"
	"mytonprovider-agent/internal/reason"
)

const (
	defaultPingTimeout = 7 * time.Second
	defaultRLDPTimeout = 10 * time.Second
)

type Checker struct {
	logger                 *slog.Logger
	prv                    ed25519.PrivateKey
	maxConcurrentProviders int
}

func New(maxConcurrentProviders int, logger *slog.Logger) (*Checker, error) {
	_, prv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}

	return &Checker{
		logger:                 logger,
		prv:                    prv,
		maxConcurrentProviders: maxConcurrentProviders,
	}, nil
}

func (c *Checker) Run(
	ctx context.Context,
	providers []*providerchecksv1.ProviderBatch,
	timeouts *providerchecksv1.CheckTimeouts,
) []*providerchecksv1.ContractCheckResult {
	if len(providers) == 0 {
		return nil
	}

	out := make(chan []*providerchecksv1.ContractCheckResult, len(providers))
	sem := make(chan struct{}, c.maxConcurrentProviders)
	var wg sync.WaitGroup

	for _, provider := range providers {
		if provider == nil {
			continue
		}
		wg.Add(1)

		go func(provider *providerchecksv1.ProviderBatch) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			log := c.logger.With("provider_pubkey", provider.ProviderPubkey)
			out <- c.checkProviderFiles(ctx, provider, timeouts, log)
		}(provider)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	results := make([]*providerchecksv1.ContractCheckResult, 0)
	for batch := range out {
		results = append(results, batch...)
	}

	return results
}

func (c *Checker) checkProviderFiles(
	ctx context.Context,
	provider *providerchecksv1.ProviderBatch,
	timeouts *providerchecksv1.CheckTimeouts,
	log *slog.Logger,
) []*providerchecksv1.ContractCheckResult {
	log.Debug("start checking provider files")
	started := time.Now()
	defer func() {
		log.Debug("finished checking provider files", "duration", time.Since(started).String())
	}()

	contracts := provider.GetContracts()
	if len(contracts) == 0 {
		return nil
	}

	stats := make(map[reason.Code]int)
	results := make([]*providerchecksv1.ContractCheckResult, 0, len(contracts))

	// To skip dead providers and save time.
	maxFailureThreshold := uint32(float32(len(contracts)) / 100.0 * 20.0)
	var failsInARow uint32

	gw := adnl.NewGateway(c.prv)
	defer gw.Close()
	if err := gw.StartClient(); err != nil {
		log.Debug("failed to start ADNL gateway", "error", err)
		return fillProviderResults(provider, contracts, reason.CantCreatePeer)
	}

	endpoint := provider.GetStorageEndpoint()
	addr := endpoint.GetIp() + ":" + strconv.Itoa(int(endpoint.GetPort()))
	peer, err := gw.RegisterClient(addr, ed25519.PublicKey(endpoint.GetAdnlPubkey()))
	if err != nil {
		log.Debug("failed to create ADNL peer", "error", err)
		return fillProviderResults(provider, contracts, reason.CantCreatePeer)
	}

	pingCtx, pingCancel := context.WithTimeout(ctx, timeoutFromMs(timeouts.GetPingMs(), defaultPingTimeout))
	_, err = peer.Ping(pingCtx)
	pingCancel()
	if err != nil {
		log.Debug("initial provider ping failed", "error", err)
		return fillProviderResults(provider, contracts, reason.FailedInitialPing)
	}

	rl := rldp.NewClientV2(peer)
	defer rl.Close()

	for _, contract := range contracts {
		if contract == nil {
			continue
		}

		if failsInARow > maxFailureThreshold {
			results = append(results, makeResult(provider, contract, reason.UnavailableProvider, 0))
			log.Info("skip", "bag_id", contract.GetBagId())
			continue
		}

		pieceStarted := time.Now()
		reasonCode := c.checkPiece(ctx, rl, contract.GetBagId(), timeouts, log)
		latency := uint32(time.Since(pieceStarted).Milliseconds())

		results = append(results, makeResult(provider, contract, reasonCode, latency))
		stats[reasonCode]++

		if reasonCode == reason.ValidStorageProof {
			failsInARow = 0
		} else {
			failsInARow++
		}

		// Weak providers may be overloaded.
		time.Sleep(500 * time.Millisecond)
	}

	for reasonCode, count := range stats {
		log.Debug("checked provider files", "reason", int(reasonCode), "count", count)
	}

	return results
}

func (c *Checker) checkPiece(
	ctx context.Context,
	rl *rldp.RLDP,
	bagID string,
	timeouts *providerchecksv1.CheckTimeouts,
	log *slog.Logger,
) reason.Code {
	log = log.With("bag_id", bagID)

	peer, ok := rl.GetADNL().(adnl.Peer)
	if !ok {
		log.Error("failed to get ADNL peer")
		return reason.UnknownPeer
	}

	// In case connection was lost.
	peer.Reinit()
	// Peer can be closed after some time, so for extra stability we reinit before each operation if needed.
	est := time.Now()

	pingCtx, cancelPing := context.WithTimeout(ctx, timeoutFromMs(timeouts.GetPingMs(), defaultPingTimeout))
	_, err := peer.Ping(pingCtx)
	cancelPing()
	if err != nil {
		log.Debug("ping to provider failed", "error", err)
		return reason.PingFailed
	}

	bag, decodeErr := hex.DecodeString(bagID)
	if decodeErr != nil {
		log.Error("failed to decode bag ID", "error", decodeErr)
		return reason.InvalidBagID
	}

	over, err := tl.Hash(keys.PublicKeyOverlay{Key: bag})
	if err != nil {
		log.Debug("failed to hash overlay key", "error", err)
		return reason.InvalidBagID
	}

	if time.Since(est) > 5*time.Second {
		peer.Reinit()
		est = time.Now()
	}

	// Get torrent info.
	var res storage.TorrentInfoContainer
	infoCtx, cancelInfo := context.WithTimeout(ctx, timeoutFromMs(timeouts.GetRldpMs(), defaultRLDPTimeout))
	err = rl.DoQuery(infoCtx, 32<<20, overlay.WrapQuery(over, &storage.GetTorrentInfo{}), &res)
	cancelInfo()
	if err != nil {
		log.Debug("failed to get torrent info from provider", "error", err)
		return reason.GetInfoFailed
	}

	cl, err := cell.FromBOC(res.Data)
	if err != nil {
		log.Debug("failed to parse BoC of torrent info", "error", err)
		return reason.InvalidHeader
	}

	if !bytes.Equal(cl.Hash(), bag) {
		log.Debug("hash not equal bag", "hash", cl.Hash(), "bag", bag)
		return reason.InvalidHeader
	}

	var info storage.TorrentInfo
	err = tlb.LoadFromCell(&info, cl.BeginParse())
	if err != nil {
		log.Debug("failed to load torrent info from cell", "error", err)
		return reason.InvalidHeader
	}

	pieceID := int32(1)
	var piecesCount int32
	if info.PieceSize != 0 {
		piecesCount = int32(info.FileSize / uint64(info.PieceSize))
	}
	if piecesCount != 0 {
		pieceID = rand.Int31n(piecesCount)
	}

	if time.Since(est) > 5*time.Second {
		peer.Reinit()
	}

	// Get piece proof and validate.
	var piece storage.Piece
	pieceCtx, cancelPiece := context.WithTimeout(ctx, timeoutFromMs(timeouts.GetRldpMs(), defaultRLDPTimeout))
	err = rl.DoQuery(pieceCtx, 32<<20, overlay.WrapQuery(over, &storage.GetPiece{PieceID: pieceID}), &piece)
	cancelPiece()
	if err != nil {
		log.Debug("failed to get piece from provider", "error", err)
		return reason.CantGetPiece
	}

	proof, err := cell.FromBOC(piece.Proof)
	if err != nil {
		log.Debug("failed to parse BoC of piece", "error", err)
		return reason.CantParseBoC
	}

	err = cell.CheckProof(proof, info.RootHash)
	if err != nil {
		log.Debug("proof check failed", "error", err)
		return reason.ProofCheckFailed
	}

	return reason.ValidStorageProof
}

func fillProviderResults(
	provider *providerchecksv1.ProviderBatch,
	contracts []*providerchecksv1.ContractRef,
	reasonCode reason.Code,
) []*providerchecksv1.ContractCheckResult {
	results := make([]*providerchecksv1.ContractCheckResult, 0, len(contracts))
	for _, contract := range contracts {
		if contract == nil {
			continue
		}
		results = append(results, makeResult(provider, contract, reasonCode, 0))
	}
	return results
}

func makeResult(
	provider *providerchecksv1.ProviderBatch,
	contract *providerchecksv1.ContractRef,
	reasonCode reason.Code,
	latencyMs uint32,
) *providerchecksv1.ContractCheckResult {
	return &providerchecksv1.ContractCheckResult{
		ProviderPubkey:  provider.GetProviderPubkey(),
		ProviderAddress: provider.GetProviderAddress(),
		ContractAddress: contract.GetContractAddress(),
		BagId:           contract.GetBagId(),
		ReasonCode:      reason.ToProto(reasonCode),
		LatencyMs:       latencyMs,
		Details:         "",
	}
}

func timeoutFromMs(ms uint32, fallback time.Duration) time.Duration {
	if ms == 0 {
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
}
