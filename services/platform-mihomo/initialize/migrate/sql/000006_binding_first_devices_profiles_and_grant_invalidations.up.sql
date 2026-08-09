DROP PROCEDURE IF EXISTS migration_000006_precheck;
CREATE PROCEDURE migration_000006_precheck()
BEGIN
    DECLARE duplicate_profile_rows BIGINT DEFAULT 0;
    DECLARE device_backfill_failures BIGINT DEFAULT 0;

    SELECT COUNT(*) INTO duplicate_profile_rows
    FROM (
        SELECT binding_id, player_id, region
        FROM account_profiles
        GROUP BY binding_id, player_id, region
        HAVING COUNT(*) > 1
    ) duplicates;
    IF duplicate_profile_rows > 0 THEN
        SIGNAL SQLSTATE '45000'
            SET MESSAGE_TEXT = 'migration 000006 failed: duplicate account_profiles rows for binding_id, player_id, region';
    END IF;

    SELECT COUNT(*) INTO device_backfill_failures
    FROM device_records d
    LEFT JOIN credential_records c ON c.platform_account_id = d.platform_account_id
    WHERE c.id IS NULL;
    IF device_backfill_failures > 0 THEN
        SIGNAL SQLSTATE '45000'
            SET MESSAGE_TEXT = 'migration 000006 failed: device_records rows cannot be backfilled from credential_records';
    END IF;
END;
CALL migration_000006_precheck();
DROP PROCEDURE migration_000006_precheck;

ALTER TABLE device_records
    ADD COLUMN binding_id BIGINT UNSIGNED NULL AFTER id;

UPDATE device_records d
JOIN credential_records c ON c.platform_account_id = d.platform_account_id
SET d.binding_id = c.binding_id
WHERE d.binding_id IS NULL;

ALTER TABLE device_records
    MODIFY COLUMN binding_id BIGINT UNSIGNED NOT NULL,
    DROP INDEX uniq_device_record,
    ADD UNIQUE KEY uniq_device_record_binding (binding_id, device_id),
    ADD KEY idx_device_binding_id (binding_id);

ALTER TABLE account_profiles
    DROP INDEX uniq_platform_profile,
    ADD UNIQUE KEY uniq_profile_binding_player_region (binding_id, player_id, region);

CREATE TABLE IF NOT EXISTS consumer_grant_invalidations (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    binding_id BIGINT UNSIGNED NOT NULL,
    consumer VARCHAR(64) NOT NULL,
    minimum_grant_version BIGINT UNSIGNED NOT NULL,
    invalidated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_consumer_grant_invalidations_binding_consumer (binding_id, consumer)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
