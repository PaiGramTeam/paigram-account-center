DROP TABLE IF EXISTS consumer_grant_invalidations;

DROP INDEX IF EXISTS uniq_profile_binding_player_region;
CREATE UNIQUE INDEX uniq_platform_profile ON account_profiles (platform_account_id, player_id, region);

DROP INDEX IF EXISTS idx_device_binding_id;
DROP INDEX IF EXISTS uniq_device_record_binding;
CREATE UNIQUE INDEX uniq_device_record ON device_records (platform_account_id, device_id);
ALTER TABLE device_records DROP COLUMN IF EXISTS binding_id;
