DROP PROCEDURE IF EXISTS migration_000007_rollback_precheck;
CREATE PROCEDURE migration_000007_rollback_precheck()
BEGIN
    DECLARE duplicate_platform_runtime_artifacts BIGINT DEFAULT 0;

    SELECT COUNT(*) INTO duplicate_platform_runtime_artifacts
    FROM (
        SELECT platform_account_id, artifact_type, scope_key
        FROM runtime_artifacts
        GROUP BY platform_account_id, artifact_type, scope_key
        HAVING COUNT(*) > 1
    ) duplicates;
    IF duplicate_platform_runtime_artifacts > 0 THEN
        SIGNAL SQLSTATE '45000'
            SET MESSAGE_TEXT = 'migration 000007 rollback failed: runtime_artifacts rows would violate platform_account_id uniqueness';
    END IF;
END;
CALL migration_000007_rollback_precheck();
DROP PROCEDURE migration_000007_rollback_precheck;

ALTER TABLE runtime_artifacts
    DROP INDEX uniq_runtime_artifact_binding,
    DROP INDEX idx_runtime_binding_id,
    DROP COLUMN binding_id,
    ADD UNIQUE KEY uniq_runtime_artifact (platform_account_id, artifact_type, scope_key);

ALTER TABLE account_profiles
    DROP INDEX uniq_default_profile_per_binding,
    DROP COLUMN default_profile_marker;
