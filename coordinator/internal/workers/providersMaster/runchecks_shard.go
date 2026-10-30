package providersmaster

import (
	"sort"

	providerchecksv1 "github.com/rudolfkova/mytonprovider-backend/contracts/gen/go/providerchecks/v1"
)

type providerAssignment struct {
	byEndpoint map[string][]*providerchecksv1.ProviderBatch
	byProvider map[string]string // providerPubkey -> endpoint
}

// assignProvidersToAgents shards ProviderBatches across agents by bag count.
// sticky maps providerPubkey -> preferred endpoint (last Valid).
// exclude maps providerPubkey -> endpoint that must not receive the provider
// when at least one other agent is available (used on retry).
func assignProvidersToAgents(
	providers []*providerchecksv1.ProviderBatch,
	endpoints []string,
	sticky map[string]string,
	exclude map[string]string,
) providerAssignment {
	out := providerAssignment{
		byEndpoint: make(map[string][]*providerchecksv1.ProviderBatch, len(endpoints)),
		byProvider: make(map[string]string, len(providers)),
	}
	if len(providers) == 0 || len(endpoints) == 0 {
		return out
	}

	for _, ep := range endpoints {
		out.byEndpoint[ep] = nil
	}

	ordered := append([]*providerchecksv1.ProviderBatch(nil), providers...)
	sort.SliceStable(ordered, func(i, j int) bool {
		ci := len(ordered[i].GetContracts())
		cj := len(ordered[j].GetContracts())
		if ci != cj {
			return ci > cj
		}
		return ordered[i].GetProviderPubkey() < ordered[j].GetProviderPubkey()
	})

	loads := make(map[string]int, len(endpoints))
	for _, ep := range endpoints {
		loads[ep] = 0
	}

	for _, batch := range ordered {
		if batch == nil {
			continue
		}
		pubkey := batch.GetProviderPubkey()
		bags := len(batch.GetContracts())
		excluded := ""
		if exclude != nil {
			excluded = exclude[pubkey]
		}

		candidates := endpoints
		if excluded != "" && len(endpoints) > 1 {
			filtered := make([]string, 0, len(endpoints)-1)
			for _, ep := range endpoints {
				if ep != excluded {
					filtered = append(filtered, ep)
				}
			}
			if len(filtered) > 0 {
				candidates = filtered
			}
		}

		chosen := pickAgent(candidates, loads, sticky[pubkey])
		out.byEndpoint[chosen] = append(out.byEndpoint[chosen], batch)
		out.byProvider[pubkey] = chosen
		loads[chosen] += bags
	}

	return out
}

func pickAgent(candidates []string, loads map[string]int, stickyEndpoint string) string {
	if stickyEndpoint != "" {
		for _, ep := range candidates {
			if ep == stickyEndpoint {
				return ep
			}
		}
	}

	chosen := candidates[0]
	minLoad := loads[chosen]
	for _, ep := range candidates[1:] {
		if loads[ep] < minLoad {
			chosen = ep
			minLoad = loads[ep]
		}
	}
	return chosen
}
