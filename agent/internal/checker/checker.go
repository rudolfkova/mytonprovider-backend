package checker

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xssnick/tonutils-go/adnl"
	"github.com/xssnick/tonutils-go/adnl/keys"
	"github.com/xssnick/tonutils-go/adnl/overlay"
	"github.com/xssnick/tonutils-go/adnl/rldp"
	"github.com/xssnick/tonutils-go/tl"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/tvm/cell"

	providerchecksv1 "mytonprovider-contracts/gen/go/providerchecks/v1"
	"mytonprovider-agent/internal/reason"
	"mytonprovider-agent/internal/tonstorage"
)

const (
	defaultPingTimeout          = 7 * time.Second
	defaultRLDPTimeout          = 2 * time.Second
	skipFailureThresholdPercent = 20.0
	pingCacheTTL                = 2 * time.Second
	interBagDelay                = 100 * time.Millisecond
)

type Checker struct {
	logger                 *slog.Logger
	prv                    ed25519.PrivateKey
	maxConcurrentProviders int
}

type jobProgress struct {
	total     uint64
	step      uint64
	logger    *slog.Logger
	processed atomic.Uint64
	nextLogAt atomic.Uint64
}

type peerPingState struct {
	lastSuccessAt time.Time
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
	baseLog *slog.Logger,
) []*providerchecksv1.ContractCheckResult {
	if len(providers) == 0 {
		return nil
	}
	if baseLog == nil {
		baseLog = c.logger
	}
	totalContracts := 0
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		totalContracts += len(provider.GetContracts())
	}
	progress := newJobProgress(uint64(totalContracts), 100, baseLog)
	if progress.total > 0 {
		baseLog.Info("job progress", "contracts_scanned", 0, "contracts_remaining", progress.total, "contracts_total", progress.total, "progress_percent", "0.0%")
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

			log := baseLog.With(
				"provider_pubkey", provider.ProviderPubkey,
				"provider_address", provider.ProviderAddress,
				"storage_ip", provider.GetStorageEndpoint().GetIp(),
				"storage_port", provider.GetStorageEndpoint().GetPort(),
			)
			out <- c.checkProviderFiles(ctx, provider, timeouts, progress, log)
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
	progress *jobProgress,
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
	detailsStats := make(map[string]int)
	results := make([]*providerchecksv1.ContractCheckResult, 0, len(contracts))

	// Keep the same dead-provider short-circuit as the old coordinator.
	maxFailureThreshold := uint32(float32(len(contracts)) / 100.0 * skipFailureThresholdPercent)
	var failsInARow uint32

	gw := adnl.NewGateway(c.prv)
	defer gw.Close()
	if err := gw.StartClient(); err != nil {
		log.Debug("failed to start ADNL gateway", "error", err)
		results := fillProviderResults(provider, contracts, reason.CantCreatePeer, fmt.Sprintf("stage=start_client error=%s", err.Error()))
		if progress != nil {
			progress.advance(uint64(len(results)))
		}
		return results
	}

	endpoint := provider.GetStorageEndpoint()
	addr := endpoint.GetIp() + ":" + strconv.Itoa(int(endpoint.GetPort()))
	peer, err := gw.RegisterClient(addr, ed25519.PublicKey(endpoint.GetAdnlPubkey()))
	if err != nil {
		log.Debug("failed to create ADNL peer", "error", err)
		results := fillProviderResults(provider, contracts, reason.CantCreatePeer, fmt.Sprintf("stage=register_client endpoint=%s error=%s", addr, err.Error()))
		if progress != nil {
			progress.advance(uint64(len(results)))
		}
		return results
	}

	pingCtx, pingCancel := context.WithTimeout(ctx, timeoutFromMs(timeouts.GetPingMs(), defaultPingTimeout))
	_, err = peer.Ping(pingCtx)
	pingCancel()
	if err != nil {
		log.Debug("initial provider ping failed", "error", err)
		results := fillProviderResults(provider, contracts, reason.FailedInitialPing, fmt.Sprintf("stage=initial_ping endpoint=%s error=%s", addr, err.Error()))
		if progress != nil {
			progress.advance(uint64(len(results)))
		}
		return results
	}

	rl := rldp.NewClientV2(peer)
	defer rl.Close()
	pingState := &peerPingState{lastSuccessAt: time.Now()}

	for _, contract := range contracts {
		if contract == nil {
			continue
		}

		if failsInARow > maxFailureThreshold {
			details := fmt.Sprintf("stage=skip reason=too_many_consecutive_failures threshold=%d fails_in_a_row=%d", maxFailureThreshold, failsInARow)
			results = append(results, makeResult(provider, contract, reason.UnavailableProvider, 0, details))
			if progress != nil {
				progress.advance(1)
			}
			log.Info("skip", "bag_id", contract.GetBagId(), "reason_code", reasonName(reason.UnavailableProvider), "details", details)
			detailsStats[details]++
			continue
		}

		pieceStarted := time.Now()
		reasonCode, details := c.checkPiece(ctx, rl, contract.GetBagId(), timeouts, pingState, log)
		latency := uint32(time.Since(pieceStarted).Milliseconds())

		results = append(results, makeResult(provider, contract, reasonCode, latency, details))
		if progress != nil {
			progress.advance(1)
		}
		stats[reasonCode]++
		if reasonCode != reason.ValidStorageProof {
			detailsStats[details]++
			log.Warn(
				"contract check failed",
				"bag_id", contract.GetBagId(),
				"contract_address", contract.GetContractAddress(),
				"reason_code", reasonName(reasonCode),
				"latency_ms", latency,
				"details", details,
			)
		}

		if reasonCode == reason.ValidStorageProof {
			failsInARow = 0
		} else {
			failsInARow++
		}

		// Keep a small pacing gap to reduce burst load.
		time.Sleep(interBagDelay)
	}

	for reasonCode, count := range stats {
		log.Debug("checked provider files", "reason", reasonName(reasonCode), "count", count)
	}
	log.Info("provider check summary", "contracts_total", len(contracts), "reason_counts", stats, "error_signatures", detailsStats)

	return results
}

func (c *Checker) checkPiece(
	ctx context.Context,
	rl *rldp.RLDP,
	bagID string,
	timeouts *providerchecksv1.CheckTimeouts,
	pingState *peerPingState,
	log *slog.Logger,
) (reason.Code, string) {
	log = log.With("bag_id", bagID)

	peer, ok := rl.GetADNL().(adnl.Peer)
	if !ok {
		log.Error("failed to get ADNL peer")
		if pingState != nil {
			pingState.invalidate()
		}
		return reason.UnknownPeer, "stage=get_adnl_peer error=adnl peer cast failed"
	}

	// In case connection was lost.
	peer.Reinit()
	// Peer can be closed after some time, so for extra stability we reinit before each operation if needed.
	est := time.Now()

	pingTimeout := timeoutFromMs(timeouts.GetPingMs(), defaultPingTimeout)
	if pingState == nil || pingState.shouldProbe(time.Now()) {
		pingCtx, pingCancel := context.WithTimeout(ctx, pingTimeout)
		_, err := peer.Ping(pingCtx)
		pingCancel()
		if err != nil {
			log.Debug("ping to provider failed", "error", err)
			if pingState != nil {
				pingState.invalidate()
			}
			return reason.PingFailed, fmt.Sprintf("stage=piece_ping timeout_ms=%d error=%s", pingTimeout.Milliseconds(), err.Error())
		}
		if pingState != nil {
			pingState.markSuccess(time.Now())
		}
	}

	bag, decodeErr := hex.DecodeString(bagID)
	if decodeErr != nil {
		log.Error("failed to decode bag ID", "error", decodeErr)
		return reason.InvalidBagID, fmt.Sprintf("stage=decode_bag_id bag_id=%s error=%s", bagID, decodeErr.Error())
	}

	over, err := tl.Hash(keys.PublicKeyOverlay{Key: bag})
	if err != nil {
		log.Debug("failed to hash overlay key", "error", err)
		return reason.InvalidBagID, fmt.Sprintf("stage=hash_overlay error=%s", err.Error())
	}

	if time.Since(est) > 5*time.Second {
		peer.Reinit()
		est = time.Now()
	}

	// Get torrent info.
	var res tonstorage.TorrentInfoContainer
	rldpTimeout := timeoutFromMs(timeouts.GetRldpMs(), defaultRLDPTimeout)
	infoCtx, cancelInfo := context.WithTimeout(ctx, rldpTimeout)
	err = rl.DoQuery(infoCtx, 32<<20, overlay.WrapQuery(over, &tonstorage.GetTorrentInfo{}), &res)
	cancelInfo()
	if err != nil {
		log.Debug("failed to get torrent info from provider", "error", err)
		if pingState != nil {
			pingState.invalidate()
		}
		return reason.GetInfoFailed, fmt.Sprintf("stage=get_torrent_info timeout_ms=%d error=%s", rldpTimeout.Milliseconds(), err.Error())
	}

	cl, err := cell.FromBOC(res.Data)
	if err != nil {
		log.Debug("failed to parse BoC of torrent info", "error", err)
		return reason.InvalidHeader, fmt.Sprintf("stage=parse_torrent_info_boc error=%s", err.Error())
	}

	if !bytes.Equal(cl.Hash(), bag) {
		log.Debug("hash not equal bag", "hash", cl.Hash(), "bag", bag)
		return reason.InvalidHeader, fmt.Sprintf("stage=validate_torrent_info_hash got=%x expected=%x", cl.Hash(), bag)
	}

	var info tonstorage.TorrentInfo
	err = tlb.Parse(&info, cl)
	if err != nil {
		log.Debug("failed to load torrent info from cell", "error", err)
		return reason.InvalidHeader, fmt.Sprintf("stage=load_torrent_info_cell error=%s", err.Error())
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
	var piece tonstorage.Piece
	pieceCtx, cancelPiece := context.WithTimeout(ctx, rldpTimeout)
	err = rl.DoQuery(pieceCtx, 32<<20, overlay.WrapQuery(over, &tonstorage.GetPiece{PieceID: pieceID}), &piece)
	cancelPiece()
	if err != nil {
		log.Debug("failed to get piece from provider", "error", err)
		if pingState != nil {
			pingState.invalidate()
		}
		return reason.CantGetPiece, fmt.Sprintf("stage=get_piece piece_id=%d timeout_ms=%d error=%s", pieceID, rldpTimeout.Milliseconds(), err.Error())
	}

	proof, err := cell.FromBOC(piece.Proof)
	if err != nil {
		log.Debug("failed to parse BoC of piece", "error", err)
		return reason.CantParseBoC, fmt.Sprintf("stage=parse_piece_proof_boc error=%s", err.Error())
	}

	err = cell.CheckProof(proof, info.RootHash)
	if err != nil {
		log.Debug("proof check failed", "error", err)
		return reason.ProofCheckFailed, fmt.Sprintf("stage=check_proof root_hash=%x error=%s", info.RootHash, err.Error())
	}
	if pingState != nil {
		pingState.markSuccess(time.Now())
	}

	return reason.ValidStorageProof, ""
}

func (s *peerPingState) shouldProbe(now time.Time) bool {
	if s == nil || s.lastSuccessAt.IsZero() {
		return true
	}
	return now.Sub(s.lastSuccessAt) >= pingCacheTTL
}

func (s *peerPingState) markSuccess(now time.Time) {
	if s == nil {
		return
	}
	s.lastSuccessAt = now
}

func (s *peerPingState) invalidate() {
	if s == nil {
		return
	}
	s.lastSuccessAt = time.Time{}
}

func fillProviderResults(
	provider *providerchecksv1.ProviderBatch,
	contracts []*providerchecksv1.ContractRef,
	reasonCode reason.Code,
	details string,
) []*providerchecksv1.ContractCheckResult {
	results := make([]*providerchecksv1.ContractCheckResult, 0, len(contracts))
	for _, contract := range contracts {
		if contract == nil {
			continue
		}
		results = append(results, makeResult(provider, contract, reasonCode, 0, details))
	}
	return results
}

func makeResult(
	provider *providerchecksv1.ProviderBatch,
	contract *providerchecksv1.ContractRef,
	reasonCode reason.Code,
	latencyMs uint32,
	details string,
) *providerchecksv1.ContractCheckResult {
	return &providerchecksv1.ContractCheckResult{
		ProviderPubkey:  provider.GetProviderPubkey(),
		ProviderAddress: provider.GetProviderAddress(),
		ContractAddress: contract.GetContractAddress(),
		BagId:           contract.GetBagId(),
		ReasonCode:      reason.ToProto(reasonCode),
		LatencyMs:       latencyMs,
		Details:         details,
	}
}

func reasonName(code reason.Code) string {
	return reason.ToProto(code).String()
}

func newJobProgress(total uint64, step uint64, logger *slog.Logger) *jobProgress {
	if step == 0 {
		step = 100
	}
	p := &jobProgress{
		total:  total,
		step:   step,
		logger: logger,
	}
	if total > 0 {
		next := step
		if next > total {
			next = total
		}
		p.nextLogAt.Store(next)
	}
	return p
}

func (p *jobProgress) advance(n uint64) {
	if p == nil || p.total == 0 || n == 0 {
		return
	}
	current := p.processed.Add(n)
	p.maybeLog(current)
}

func (p *jobProgress) maybeLog(current uint64) {
	for {
		next := p.nextLogAt.Load()
		if next == 0 || (current < next && current < p.total) {
			return
		}
		newNext := next + p.step
		if newNext > p.total {
			newNext = p.total
		}
		if next >= p.total {
			newNext = 0
		}
		if !p.nextLogAt.CompareAndSwap(next, newNext) {
			continue
		}
		scanned := current
		if scanned > p.total {
			scanned = p.total
		}
		remaining := p.total - scanned
		p.logger.Info(
			"job progress",
			"contracts_scanned", scanned,
			"contracts_remaining", remaining,
			"contracts_total", p.total,
			"progress_percent", fmt.Sprintf("%.1f%%", float64(scanned)*100.0/float64(p.total)),
		)
		return
	}
}

func timeoutFromMs(ms uint32, fallback time.Duration) time.Duration {
	if ms == 0 {
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
}
