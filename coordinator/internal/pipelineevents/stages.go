package pipelineevents

const (
	StageEndpointResolve      = "endpoint_resolve"
	StageEndpointUnresolved   = "endpoint_unresolved"
	StageRecovered              = "recovered"
	StageUnknown                = "unknown"
	StageAgentRunChecksNoResult   = "agent_runchecks_no_result"
	StageAgentRunChecksUnavailable = "agent_runchecks_unavailable"

	StageDHTFindProviderRecord    = "dht_find_provider_record"
	StageDHTFindProviderAddresses = "dht_find_provider_addresses"
	StageVerifyStorageADNLProof   = "verify_storage_adnl_proof"
	StageDHTFindStorageAddresses  = "dht_find_storage_addresses"
	StageOverlayStorageFallback   = "overlay_storage_fallback"
)
