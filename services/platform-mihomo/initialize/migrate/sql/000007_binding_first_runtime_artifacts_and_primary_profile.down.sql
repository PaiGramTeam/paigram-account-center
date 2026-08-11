DO $$
DECLARE
    duplicate_platform_runtime_artifacts BIGINT;
BEGIN
    SELECT COUNT(*) INTO duplicate_platform_runtime_artifacts
    FROM (
        SELECT platform_account_id, artifact_type, scope_key
        FROM runtime_artifacts
        GROUP BY platform_account_id, artifact_type, scope_key
        HAVING COUNT(*) > 1
    ) AS duplicates;
    IF duplicate_platform_runtime_artifacts > 0 THEN
        RAISE EXCEPTION 'migration 000007 rollback failed: runtime_artifacts rows would violate platform_account_id uniqueness'
            USING ERRCODE = '23514';
    END IF;
END
$$;

DROP INDEX IF EXISTS uniq_runtime_artifact_binding;
DROP INDEX IF EXISTS idx_runtime_binding_id;
ALTER TABLE runtime_artifacts DROP COLUMN IF EXISTS binding_id;
CREATE UNIQUE INDEX uniq_runtime_artifact
    ON runtime_artifacts (platform_account_id, artifact_type, scope_key);

DROP INDEX IF EXISTS uniq_default_profile_per_binding;
ALTER TABLE account_profiles DROP COLUMN IF EXISTS default_profile_marker;
