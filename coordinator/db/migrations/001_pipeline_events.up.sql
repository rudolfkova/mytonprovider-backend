-- Pipeline error/ok event log for provider and bag triage.
-- Run manually on production BEFORE deploying coordinator with pipeline events support.

CREATE TABLE IF NOT EXISTS providers.provider_pipeline_events
(
    id              bigserial PRIMARY KEY,
    provider_pubkey character varying(64) NOT NULL,
    status          character varying(8) NOT NULL CHECK (status IN ('error', 'ok')),
    stage           character varying(64) NOT NULL,
    reason_code     integer,
    error_message   text,
    run_id          character varying(128) NOT NULL,
    worker          character varying(64) NOT NULL DEFAULT 'StoreProof',
    created_at      timestamp with time zone NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_provider_pipeline_events_pubkey_created
    ON providers.provider_pipeline_events (provider_pubkey, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_provider_pipeline_events_created
    ON providers.provider_pipeline_events (created_at);

CREATE TABLE IF NOT EXISTS providers.bag_pipeline_events
(
    id                bigserial PRIMARY KEY,
    provider_pubkey   character varying(64) NOT NULL,
    contract_address  character varying(64) NOT NULL,
    bag_id            character varying(64) NOT NULL,
    status            character varying(8) NOT NULL CHECK (status IN ('error', 'ok')),
    stage             character varying(64) NOT NULL,
    reason_code       integer,
    error_message     text,
    run_id            character varying(128) NOT NULL,
    worker            character varying(64) NOT NULL DEFAULT 'StoreProof',
    created_at        timestamp with time zone NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_bag_pipeline_events_provider_created
    ON providers.bag_pipeline_events (provider_pubkey, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_bag_pipeline_events_bag_created
    ON providers.bag_pipeline_events (bag_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_bag_pipeline_events_contract_provider_created
    ON providers.bag_pipeline_events (contract_address, provider_pubkey, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_bag_pipeline_events_created
    ON providers.bag_pipeline_events (created_at);
