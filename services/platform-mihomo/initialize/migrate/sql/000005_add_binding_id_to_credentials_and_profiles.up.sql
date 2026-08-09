DROP PROCEDURE IF EXISTS migration_000005_precheck;
CREATE PROCEDURE migration_000005_precheck()
BEGIN
    DECLARE invalid_credential_account_ids BIGINT DEFAULT 0;
    DECLARE duplicate_parsed_credential_binding_ids BIGINT DEFAULT 0;
    DECLARE invalid_profile_account_ids BIGINT DEFAULT 0;

    SELECT COUNT(*) INTO invalid_credential_account_ids
    FROM credential_records
    WHERE platform_account_id IS NULL
       OR NOT (
           platform_account_id REGEXP '^binding_[1-9][0-9]*_.+$'
           OR platform_account_id REGEXP '^hoyo_ref_[1-9][0-9]*_.+$'
       );
    IF invalid_credential_account_ids > 0 THEN
        SIGNAL SQLSTATE '45000'
            SET MESSAGE_TEXT = 'migration 000005 failed: malformed credential_records platform_account_id values';
    END IF;

    SELECT COUNT(*) INTO duplicate_parsed_credential_binding_ids
    FROM (
        SELECT CASE
            WHEN platform_account_id REGEXP '^binding_[1-9][0-9]*_.+$' THEN CAST(SUBSTRING_INDEX(SUBSTRING_INDEX(platform_account_id, '_', 2), '_', -1) AS UNSIGNED)
            WHEN platform_account_id REGEXP '^hoyo_ref_[1-9][0-9]*_.+$' THEN CAST(SUBSTRING_INDEX(SUBSTRING_INDEX(platform_account_id, '_', 3), '_', -1) AS UNSIGNED)
        END AS parsed_binding_id
        FROM credential_records
        WHERE platform_account_id REGEXP '^binding_[1-9][0-9]*_.+$'
           OR platform_account_id REGEXP '^hoyo_ref_[1-9][0-9]*_.+$'
        GROUP BY parsed_binding_id
        HAVING COUNT(*) > 1
    ) duplicate_credential_binding_ids;
    IF duplicate_parsed_credential_binding_ids > 0 THEN
        SIGNAL SQLSTATE '45000'
            SET MESSAGE_TEXT = 'migration 000005 failed: duplicate parsed credential_records binding_id values';
    END IF;

    SELECT COUNT(*) INTO invalid_profile_account_ids
    FROM account_profiles
    WHERE platform_account_id IS NULL
       OR NOT (
           platform_account_id REGEXP '^binding_[1-9][0-9]*_.+$'
           OR platform_account_id REGEXP '^hoyo_ref_[1-9][0-9]*_.+$'
       );
    IF invalid_profile_account_ids > 0 THEN
        SIGNAL SQLSTATE '45000'
            SET MESSAGE_TEXT = 'migration 000005 failed: malformed account_profiles platform_account_id values';
    END IF;
END;
CALL migration_000005_precheck();
DROP PROCEDURE migration_000005_precheck;

ALTER TABLE credential_records
    ADD COLUMN binding_id BIGINT UNSIGNED NULL AFTER id;

UPDATE credential_records
SET binding_id = CASE
    WHEN platform_account_id REGEXP '^binding_[1-9][0-9]*_.+$' THEN CAST(SUBSTRING_INDEX(SUBSTRING_INDEX(platform_account_id, '_', 2), '_', -1) AS UNSIGNED)
    WHEN platform_account_id REGEXP '^hoyo_ref_[1-9][0-9]*_.+$' THEN CAST(SUBSTRING_INDEX(SUBSTRING_INDEX(platform_account_id, '_', 3), '_', -1) AS UNSIGNED)
END
WHERE binding_id IS NULL;

ALTER TABLE credential_records
    MODIFY COLUMN binding_id BIGINT UNSIGNED NOT NULL,
    ADD UNIQUE KEY uniq_credential_binding_id (binding_id);

ALTER TABLE account_profiles
    ADD COLUMN binding_id BIGINT UNSIGNED NULL AFTER id;

UPDATE account_profiles
SET binding_id = CASE
    WHEN platform_account_id REGEXP '^binding_[1-9][0-9]*_.+$' THEN CAST(SUBSTRING_INDEX(SUBSTRING_INDEX(platform_account_id, '_', 2), '_', -1) AS UNSIGNED)
    WHEN platform_account_id REGEXP '^hoyo_ref_[1-9][0-9]*_.+$' THEN CAST(SUBSTRING_INDEX(SUBSTRING_INDEX(platform_account_id, '_', 3), '_', -1) AS UNSIGNED)
END
WHERE binding_id IS NULL;

ALTER TABLE account_profiles
    MODIFY COLUMN binding_id BIGINT UNSIGNED NOT NULL,
    ADD KEY idx_profile_binding_id (binding_id);
