ALTER TABLE account_profiles
    DROP INDEX idx_profile_binding_id,
    DROP COLUMN binding_id;

ALTER TABLE credential_records
    DROP INDEX uniq_credential_binding_id,
    DROP COLUMN binding_id;
