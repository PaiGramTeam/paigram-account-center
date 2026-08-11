CREATE TABLE IF NOT EXISTS device_records (
    id BIGSERIAL PRIMARY KEY,
    platform_account_id VARCHAR(64) NOT NULL,
    device_id VARCHAR(64) NOT NULL,
    device_fp VARCHAR(64) NOT NULL,
    device_name VARCHAR(128),
    is_valid BOOLEAN NOT NULL DEFAULT TRUE,
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uniq_device_record ON device_records (platform_account_id, device_id);
CREATE INDEX idx_device_platform_account_id ON device_records (platform_account_id);
CREATE INDEX idx_device_device_id ON device_records (device_id);
