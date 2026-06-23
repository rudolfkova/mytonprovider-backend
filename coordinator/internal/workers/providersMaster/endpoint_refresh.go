package providersmaster

import (
	"context"
	"crypto/ed25519"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"mytonprovider-coordinator/internal/models/db"
)

var (
	endpointRefreshMetricsOnce sync.Once

	endpointRefreshAttemptsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "ton_storage",
			Subsystem: "mtpo_workers",
			Name:      "endpoint_refresh_attempts_total",
			Help:      "Endpoint refresh attempts grouped by mode and result",
		},
		[]string{"mode", "result"},
	)

	staleProvidersGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "ton_storage",
			Subsystem: "mtpo_workers",
			Name:      "endpoint_stale_providers",
			Help:      "Count of stale providers in current refresh pass",
		},
		[]string{"mode"},
	)

	staleRelationsGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "ton_storage",
			Subsystem: "mtpo_workers",
			Name:      "endpoint_stale_relations",
			Help:      "Count of stale relations in current refresh pass",
		},
		[]string{"mode"},
	)
)

func initEndpointRefreshMetrics() {
	endpointRefreshMetricsOnce.Do(func() {
		prometheus.MustRegister(endpointRefreshAttemptsTotal, staleProvidersGauge, staleRelationsGauge)
	})
}

func hasValidResolvedStorageEndpoint(ip db.ProviderIP) bool {
	return strings.TrimSpace(ip.Storage.IP) != "" && ip.Storage.Port > 0 && len(ip.Storage.PublicKey) == ed25519.PublicKeySize
}

func isEndpointStateStale(state db.ProviderEndpointState, ttl time.Duration, now time.Time) bool {
	if strings.TrimSpace(state.StorageIP) == "" || state.StoragePort <= 0 {
		return true
	}
	if state.UpdatedAt == nil {
		return true
	}
	if ttl <= 0 {
		return false
	}
	return now.Sub(*state.UpdatedAt) > ttl
}

func (w *providersMasterWorker) getLastKnownIP(pubkey string) (db.ProviderIP, bool) {
	w.lastIPsMu.RLock()
	defer w.lastIPsMu.RUnlock()
	ip, ok := w.lastKnownIPs[pubkey]
	return ip, ok
}

func (w *providersMasterWorker) setLastKnownIP(pubkey string, ip db.ProviderIP) {
	w.lastIPsMu.Lock()
	w.lastKnownIPs[pubkey] = ip
	w.lastIPsMu.Unlock()
}

func (w *providersMasterWorker) RefreshEndpointsFull(ctx context.Context) (interval time.Duration, err error) {
	const fallbackFailureInterval = 30 * time.Second

	successInterval := w.endpointCfg.FullRefreshInterval
	if successInterval <= 0 {
		successInterval = defaultEndpointFullRefreshInterval
	}
	failureInterval := w.endpointCfg.FailInterval
	if failureInterval <= 0 {
		failureInterval = fallbackFailureInterval
	}

	log := w.logger.With("worker", "RefreshEndpointsFull")
	interval = successInterval

	storageContracts, err := w.providers.GetStorageContracts(ctx)
	if err != nil {
		log.Error("failed to get storage contracts for full endpoint refresh", "error", err)
		return failureInterval, err
	}
	if len(storageContracts) == 0 {
		log.Info("no storage contracts found for full endpoint refresh")
		return successInterval, nil
	}

	refreshed, refreshErr := w.refreshProvidersEndpoints(ctx, storageContracts, false)
	if refreshErr != nil {
		log.Error("full endpoint refresh failed", "error", refreshErr)
		return failureInterval, refreshErr
	}

	log.Info("full endpoint refresh completed", "providers_with_resolved_endpoints", len(refreshed))
	return successInterval, nil
}

func (w *providersMasterWorker) refreshProvidersEndpoints(ctx context.Context, storageContracts []db.ContractToProviderRelation, staleOnly bool) (availableProvidersIPs map[string]db.ProviderIP, err error) {
	mode := "full"
	if staleOnly {
		mode = "stale"
	}
	log := w.logger.With(
		slog.String("worker", "EndpointRefresh"),
		slog.String("mode", mode),
	)

	if len(storageContracts) == 0 {
		return map[string]db.ProviderIP{}, nil
	}

	uniqueProviders := make(map[string]db.ContractToProviderRelation)
	providerContracts := make(map[string][]db.ContractToProviderRelation)
	for _, sc := range storageContracts {
		if _, exists := uniqueProviders[sc.ProviderPublicKey]; !exists {
			uniqueProviders[sc.ProviderPublicKey] = sc
		}
		providerContracts[sc.ProviderPublicKey] = append(providerContracts[sc.ProviderPublicKey], sc)
	}

	pubkeys := make([]string, 0, len(uniqueProviders))
	for pubkey := range uniqueProviders {
		pubkeys = append(pubkeys, pubkey)
	}
	stateRows, err := w.providers.GetProvidersEndpointState(ctx, pubkeys)
	if err != nil {
		return nil, err
	}
	stateByPubkey := make(map[string]db.ProviderEndpointState, len(stateRows))
	for _, row := range stateRows {
		stateByPubkey[row.PublicKey] = row
	}

	now := time.Now()
	staleProviders := 0
	staleRelations := 0
	targets := make([]db.ContractToProviderRelation, 0, len(uniqueProviders))
	for pubkey, contract := range uniqueProviders {
		state, ok := stateByPubkey[pubkey]
		isStale := !ok || isEndpointStateStale(state, w.endpointCfg.StaleTTL, now)
		if isStale {
			staleProviders++
			staleRelations += len(providerContracts[pubkey])
		}
		if staleOnly && !isStale {
			continue
		}
		targets = append(targets, contract)
	}
	staleProvidersGauge.WithLabelValues(mode).Set(float64(staleProviders))
	staleRelationsGauge.WithLabelValues(mode).Set(float64(staleRelations))

	if staleOnly && len(targets) == 0 {
		log.Info("no stale providers for pre-refresh")
		return map[string]db.ProviderIP{}, nil
	}

	availableProvidersIPs = make(map[string]db.ProviderIP, len(uniqueProviders))
	resolvedForPersist := make(map[string]db.ProviderIP, len(targets))
	for pubkey := range uniqueProviders {
		if cached, ok := w.getLastKnownIP(pubkey); ok && hasValidResolvedStorageEndpoint(cached) {
			availableProvidersIPs[pubkey] = cached
		}
	}

	notFoundIPs := make([]string, 0, len(targets))
	semaphore := make(chan struct{}, maxConcurrentProviderChecks)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, sc := range targets {
		wg.Add(1)
		go func(contract db.ContractToProviderRelation) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			endpointRefreshAttemptsTotal.WithLabelValues(mode, "attempt").Inc()
			resolved, resolveErr := w.findProviderIPs(ctx, contract, log)

			mu.Lock()
			defer mu.Unlock()
			if resolveErr != nil {
				endpointRefreshAttemptsTotal.WithLabelValues(mode, "error").Inc()
				notFoundIPs = append(notFoundIPs, contract.ProviderPublicKey)
				if hasValidResolvedStorageEndpoint(resolved) {
					availableProvidersIPs[contract.ProviderPublicKey] = resolved
				}
				return
			}

			if hasValidResolvedStorageEndpoint(resolved) {
				endpointRefreshAttemptsTotal.WithLabelValues(mode, "success").Inc()
				availableProvidersIPs[contract.ProviderPublicKey] = resolved
				resolvedForPersist[contract.ProviderPublicKey] = resolved
				w.setLastKnownIP(contract.ProviderPublicKey, resolved)
			}
		}(sc)
	}
	wg.Wait()

	for _, pk := range notFoundIPs {
		ip := availableProvidersIPs[pk]
		if strings.TrimSpace(ip.Provider.IP) == "" {
			continue
		}

		contracts := providerContracts[pk]
		if len(contracts) == 0 {
			continue
		}

		storageIP, overlayErr := w.findStorageIPOverlay(ctx, ip.Provider.IP, contracts, log)
		if overlayErr != nil {
			log.Warn("overlay fallback for storage endpoint failed", "provider_pubkey", pk, "error", overlayErr)
			continue
		}

		ip.Storage = storageIP
		if hasValidResolvedStorageEndpoint(ip) {
			availableProvidersIPs[pk] = ip
			resolvedForPersist[pk] = ip
			w.setLastKnownIP(pk, ip)
			endpointRefreshAttemptsTotal.WithLabelValues(mode, "overlay_success").Inc()
		}
	}

	if len(resolvedForPersist) > 0 {
		ips := make([]db.ProviderIP, 0, len(resolvedForPersist))
		for _, row := range resolvedForPersist {
			ips = append(ips, row)
		}
		if err = w.providers.UpdateProvidersIPs(ctx, ips); err != nil {
			return nil, err
		}
	}

	if staleOnly {
		log.Info(
			"stale-first endpoint refresh completed",
			"stale_providers", staleProviders,
			"stale_relations", staleRelations,
			"targets", len(targets),
			"resolved", len(resolvedForPersist),
		)
	} else {
		log.Info(
			"endpoint refresh completed",
			"targets", len(targets),
			"resolved", len(resolvedForPersist),
			"available_for_runchecks", len(availableProvidersIPs),
		)
	}

	return availableProvidersIPs, nil
}
