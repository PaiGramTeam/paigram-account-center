CREATE TABLE IF NOT EXISTS account_profiles (
    id BIGSERIAL PRIMARY KEY,
    platform_account_id VARCHAR(64) NOT NULL,
    game_biz VARCHAR(64) NOT NULL,
    region VARCHAR(32) NOT NULL,
    player_id VARCHAR(64) NOT NULL,
    nickname VARCHAR(255) NOT NULL,
    level INT NOT NULL DEFAULT 0,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uniq_platform_profile ON account_profiles (platform_account_id, player_id, region);
CREATE INDEX idx_profile_platform_account_id ON account_profiles (platform_account_id);
