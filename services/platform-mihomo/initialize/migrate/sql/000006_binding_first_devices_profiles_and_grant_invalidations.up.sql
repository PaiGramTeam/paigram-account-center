DO $$
DECLARE
    duplicate_profile_rows BIGINT;
    device_backfill_failures BIGINT;
BEGIN
    SELECT COUNT(*) INTO duplicate_profile_rows
    FROM (
        SELECT binding_id, player_id, region
        FROM account_profiles
        GROUP BY binding_id, player_id, region
        HAVING COUNT(*) > 1
    ) AS duplicates;
    IF duplicate_profile_rows > 0 THEN
        RAISE EXCEPTION 'migration 000006 failed: duplicate account_profiles rows for binding_id, player_id, region'
            USING ERRCODE = '23514';
    END IF;

    SELECT COUNT(*) INTO device_backfill_failures
    FROM device_records AS d
    LEFT JOIN credential_records AS c ON c.platform_account_id = d.platform_account_id
    WHERE c.id IS NULL;
    IF device_backfill_failures > 0 THEN
        RAISE EXCEPTION 'migration 000006 failed: device_records rows cannot be backfilled from credential_records'
            USING ERRCODE = '23514';
    END IF;
END
$$;

ALTER TABLE device_records ADD COLUMN binding_id BIGINT;

UPDATE device_records AS d
SET binding_id = c.binding_id
FROM credential_records AS c
WHERE c.platform_account_id = d.platform_account_id
  AND d.binding_id IS NULL;

ALTER TABLE device_records ALTER COLUMN binding_id SET NOT NULL;
DROP INDEX uniq_device_record;
CREATE UNIQUE INDEX uniq_device_record_binding ON device_records (binding_id, device_id);
CREATE INDEX idx_device_binding_id ON device_records (binding_id);

DROP INDEX uniq_platform_profile;
CREATE UNIQUE INDEX uniq_profile_binding_player_region ON account_profiles (binding_id, player_id, region);

CREATE TABLE IF NOT EXISTS consumer_grant_invalidations (
    id BIGSERIAL PRIMARY KEY,
    binding_id BIGINT NOT NULL,
    consumer VARCHAR(64) NOT NULL,
    minimum_grant_version BIGINT NOT NULL,
    invalidated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_consumer_grant_invalidations_binding_consumer UNIQUE (binding_id, consumer)
);
