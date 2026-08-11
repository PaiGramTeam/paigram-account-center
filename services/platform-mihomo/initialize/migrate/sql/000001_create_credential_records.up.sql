CREATE TABLE IF NOT EXISTS credential_records (
    id BIGSERIAL PRIMARY KEY,
    platform_account_id VARCHAR(64) NOT NULL,
    platform VARCHAR(32) NOT NULL,
    account_id VARCHAR(64) NOT NULL,
    region VARCHAR(32) NOT NULL,
    credential_blob TEXT NOT NULL,
    credential_version VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    last_validated_at TIMESTAMPTZ,
    last_refreshed_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uniq_platform_account_id UNIQUE (platform_account_id),
    CONSTRAINT uniq_platform_account UNIQUE (platform, account_id)
);
