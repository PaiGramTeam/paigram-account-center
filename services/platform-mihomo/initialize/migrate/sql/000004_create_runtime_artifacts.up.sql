CREATE TABLE IF NOT EXISTS runtime_artifacts (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    platform_account_id VARCHAR(64) NOT NULL,
    artifact_type VARCHAR(64) NOT NULL,
    artifact_value TEXT NOT NULL,
    scope_key VARCHAR(128) NOT NULL,
    expires_at DATETIME(3) NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uniq_runtime_artifact (platform_account_id, artifact_type, scope_key),
    KEY idx_runtime_platform_account_id (platform_account_id),
    KEY idx_runtime_artifact_type (artifact_type),
    KEY idx_runtime_expires_at (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
