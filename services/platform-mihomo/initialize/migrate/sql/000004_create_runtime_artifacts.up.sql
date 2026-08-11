CREATE TABLE IF NOT EXISTS runtime_artifacts (
    id BIGSERIAL PRIMARY KEY,
    platform_account_id VARCHAR(64) NOT NULL,
    artifact_type VARCHAR(64) NOT NULL,
    artifact_value TEXT NOT NULL,
    scope_key VARCHAR(128) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uniq_runtime_artifact ON runtime_artifacts (platform_account_id, artifact_type, scope_key);
CREATE INDEX idx_runtime_platform_account_id ON runtime_artifacts (platform_account_id);
CREATE INDEX idx_runtime_artifact_type ON runtime_artifacts (artifact_type);
CREATE INDEX idx_runtime_expires_at ON runtime_artifacts (expires_at);
