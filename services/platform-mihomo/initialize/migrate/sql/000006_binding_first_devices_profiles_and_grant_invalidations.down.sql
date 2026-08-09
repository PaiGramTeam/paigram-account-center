DROP TABLE IF EXISTS consumer_grant_invalidations;

ALTER TABLE account_profiles
    DROP INDEX uniq_profile_binding_player_region,
    ADD UNIQUE KEY uniq_platform_profile (platform_account_id, player_id, region);

ALTER TABLE device_records
    DROP INDEX idx_device_binding_id,
    DROP INDEX uniq_device_record_binding,
    ADD UNIQUE KEY uniq_device_record (platform_account_id, device_id),
    DROP COLUMN binding_id;
