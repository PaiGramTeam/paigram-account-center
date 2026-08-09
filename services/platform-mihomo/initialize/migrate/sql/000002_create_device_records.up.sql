CREATE TABLE IF NOT EXISTS device_records (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    platform_account_id VARCHAR(64) NOT NULL,
    device_id VARCHAR(64) NOT NULL,
    device_fp VARCHAR(64) NOT NULL,
    device_name VARCHAR(128) NULL,
    is_valid BOOLEAN NOT NULL DEFAULT TRUE,
    last_seen_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uniq_device_record (platform_account_id, device_id),
    KEY idx_device_platform_account_id (platform_account_id),
    KEY idx_device_device_id (device_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
