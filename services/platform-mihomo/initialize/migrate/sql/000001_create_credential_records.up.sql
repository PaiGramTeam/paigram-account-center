CREATE TABLE IF NOT EXISTS credential_records (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    platform_account_id VARCHAR(64) NOT NULL,
    platform VARCHAR(32) NOT NULL,
    account_id VARCHAR(64) NOT NULL,
    region VARCHAR(32) NOT NULL,
    credential_blob TEXT NOT NULL,
    credential_version VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    last_validated_at DATETIME(3) NULL,
    last_refreshed_at DATETIME(3) NULL,
    expires_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uniq_platform_account_id (platform_account_id),
    UNIQUE KEY uniq_platform_account (platform, account_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
