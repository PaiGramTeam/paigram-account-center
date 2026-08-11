DROP INDEX IF EXISTS idx_profile_binding_id;
ALTER TABLE account_profiles DROP COLUMN IF EXISTS binding_id;

DROP INDEX IF EXISTS uniq_credential_binding_id;
ALTER TABLE credential_records DROP COLUMN IF EXISTS binding_id;
