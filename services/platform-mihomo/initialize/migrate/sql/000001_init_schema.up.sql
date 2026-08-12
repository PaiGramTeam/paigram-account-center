CREATE TABLE credential_records (
    id BIGSERIAL PRIMARY KEY,
    binding_ref VARCHAR(64) NOT NULL,
    account_key VARCHAR(64) NOT NULL,
    generation BIGINT NOT NULL CHECK (generation >= 1),
    platform VARCHAR(32) NOT NULL,
    account_id VARCHAR(64) NOT NULL,
    region VARCHAR(32) NOT NULL,
    credential_blob TEXT NOT NULL,
    credential_version VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    last_validated_at TIMESTAMPTZ NULL,
    last_refreshed_at TIMESTAMPTZ NULL,
    expires_at TIMESTAMPTZ NULL,
    profile_snapshot_complete BOOLEAN NOT NULL DEFAULT FALSE,
    profile_revision BIGINT NOT NULL DEFAULT 0,
    profile_observed_revision BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT valid_credential_binding_ref CHECK (binding_ref <> ''),
    CONSTRAINT valid_credential_account_key CHECK (account_key <> ''),
    CONSTRAINT valid_profile_observation CHECK (profile_observed_revision <= profile_revision),
    CONSTRAINT uniq_credential_binding_ref UNIQUE (binding_ref),
    CONSTRAINT uniq_account_key UNIQUE (account_key),
    CONSTRAINT uniq_platform_account UNIQUE (platform, account_id),
    CONSTRAINT uniq_credential_binding_account UNIQUE (binding_ref, account_key)
);

CREATE TABLE device_records (
    id BIGSERIAL PRIMARY KEY,
    binding_ref VARCHAR(64) NOT NULL,
    account_key VARCHAR(64) NOT NULL,
    device_ref VARCHAR(64) NOT NULL,
    device_id VARCHAR(64) NOT NULL,
    device_fp VARCHAR(64) NOT NULL,
    device_name VARCHAR(128) NULL,
    is_valid BOOLEAN NOT NULL DEFAULT TRUE,
    last_seen_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_device_binding_account FOREIGN KEY (binding_ref, account_key)
        REFERENCES credential_records (binding_ref, account_key) ON DELETE CASCADE,
    CONSTRAINT uniq_device_ref UNIQUE (device_ref),
    CONSTRAINT uniq_device_record_binding UNIQUE (binding_ref, device_id)
);

CREATE INDEX idx_device_binding_ref ON device_records (binding_ref);
CREATE INDEX idx_device_account_key ON device_records (account_key);
CREATE INDEX idx_device_device_id ON device_records (device_id);

CREATE TABLE account_profiles (
    id BIGSERIAL PRIMARY KEY,
    binding_ref VARCHAR(64) NOT NULL,
    account_key VARCHAR(64) NOT NULL,
    profile_ref VARCHAR(64) NOT NULL,
    game_biz VARCHAR(64) NOT NULL,
    region VARCHAR(32) NOT NULL,
    player_id VARCHAR(64) NOT NULL,
    nickname VARCHAR(255) NOT NULL,
    level INTEGER NOT NULL DEFAULT 0,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_profile_binding_account FOREIGN KEY (binding_ref, account_key)
        REFERENCES credential_records (binding_ref, account_key) ON DELETE CASCADE,
    CONSTRAINT uniq_profile_ref UNIQUE (profile_ref),
    CONSTRAINT uniq_profile_binding_player_region UNIQUE (binding_ref, player_id, region)
);

CREATE INDEX idx_profile_binding_ref ON account_profiles (binding_ref);
CREATE INDEX idx_profile_account_key ON account_profiles (account_key);
CREATE UNIQUE INDEX uniq_default_profile_per_binding
    ON account_profiles (binding_ref) WHERE is_default;

CREATE TABLE runtime_artifacts (
    id BIGSERIAL PRIMARY KEY,
    binding_ref VARCHAR(64) NOT NULL,
    account_key VARCHAR(64) NOT NULL,
    artifact_type VARCHAR(64) NOT NULL,
    artifact_value TEXT NOT NULL,
    scope_key VARCHAR(128) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_artifact_binding_account FOREIGN KEY (binding_ref, account_key)
        REFERENCES credential_records (binding_ref, account_key) ON DELETE CASCADE,
    CONSTRAINT uniq_runtime_artifact_binding UNIQUE (binding_ref, artifact_type, scope_key)
);

CREATE INDEX idx_runtime_binding_ref ON runtime_artifacts (binding_ref);
CREATE INDEX idx_runtime_account_key ON runtime_artifacts (account_key);
CREATE INDEX idx_runtime_artifact_type ON runtime_artifacts (artifact_type);
CREATE INDEX idx_runtime_expires_at ON runtime_artifacts (expires_at);

CREATE TABLE consumer_grant_invalidations (
    id BIGSERIAL PRIMARY KEY,
    binding_ref VARCHAR(64) NOT NULL,
    consumer VARCHAR(64) NOT NULL,
    minimum_grant_version BIGINT NOT NULL CHECK (minimum_grant_version >= 1),
    invalidated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_consumer_grant_invalidations_binding_consumer UNIQUE (binding_ref, consumer)
);

CREATE TABLE authorization_fences (
    id BIGSERIAL PRIMARY KEY,
    binding_ref VARCHAR(64) NOT NULL,
    consumer_principal VARCHAR(128) NOT NULL,
    minimum_grant_version BIGINT NOT NULL,
    minimum_owner_epoch BIGINT NOT NULL,
    minimum_consumer_epoch BIGINT NOT NULL,
    minimum_entry_epoch BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uniq_authorization_fence UNIQUE (binding_ref, consumer_principal)
);

CREATE TABLE platform_operations (
    operation_id VARCHAR(128) PRIMARY KEY,
    kind VARCHAR(64) NOT NULL,
    binding_ref VARCHAR(64) NOT NULL,
    pre_generation BIGINT NOT NULL,
    target_generation BIGINT NOT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    execution_token VARCHAR(64) NOT NULL,
    lease_expires_at TIMESTAMPTZ NOT NULL,
    state VARCHAR(32) NOT NULL,
    reason_code VARCHAR(128) NOT NULL DEFAULT '',
    account_key VARCHAR(64) NOT NULL DEFAULT '',
    credential_status VARCHAR(32) NOT NULL DEFAULT '',
    snapshot_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT valid_operation_generation CHECK (
        (kind IN ('OPERATION_KIND_APPLY_AUTHORIZATION_FENCE', 'OPERATION_KIND_SET_PRIMARY_PROFILE') AND target_generation = pre_generation)
        OR (kind NOT IN ('OPERATION_KIND_APPLY_AUTHORIZATION_FENCE', 'OPERATION_KIND_SET_PRIMARY_PROFILE') AND target_generation = pre_generation + 1)
    ),
    CONSTRAINT valid_operation_request_fingerprint CHECK (request_fingerprint <> ''),
    CONSTRAINT valid_operation_execution_token CHECK (execution_token <> ''),
    CONSTRAINT valid_operation_state CHECK (state IN (
        'pending',
        'succeeded',
        'failed',
        'not_received',
        'failed_input_required'
    ))
);

CREATE INDEX idx_operation_binding_ref ON platform_operations (binding_ref);
