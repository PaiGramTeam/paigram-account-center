CREATE TABLE IF NOT EXISTS account_profiles (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    platform_account_id VARCHAR(64) NOT NULL,
    game_biz VARCHAR(64) NOT NULL,
    region VARCHAR(32) NOT NULL,
    player_id VARCHAR(64) NOT NULL,
    nickname VARCHAR(255) NOT NULL,
    level INT NOT NULL DEFAULT 0,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    discovered_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uniq_platform_profile (platform_account_id, player_id, region),
    KEY idx_profile_platform_account_id (platform_account_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
