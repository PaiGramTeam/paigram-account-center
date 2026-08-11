DO $$
DECLARE
    multiple_default_profile_bindings BIGINT;
    unmapped_runtime_artifacts BIGINT;
    duplicate_binding_runtime_artifacts BIGINT;
BEGIN
    SELECT COUNT(*) INTO multiple_default_profile_bindings
    FROM (
        SELECT binding_id
        FROM account_profiles
        WHERE is_default = TRUE
        GROUP BY binding_id
        HAVING COUNT(*) > 1
    ) AS duplicates;
    IF multiple_default_profile_bindings > 0 THEN
        RAISE EXCEPTION 'migration 000007 failed: multiple default account_profiles rows for binding_id'
            USING ERRCODE = '23514';
    END IF;

    SELECT COUNT(*) INTO unmapped_runtime_artifacts
    FROM runtime_artifacts AS ra
    LEFT JOIN credential_records AS cr ON cr.platform_account_id = ra.platform_account_id
    WHERE cr.binding_id IS NULL;
    IF unmapped_runtime_artifacts > 0 THEN
        RAISE EXCEPTION 'migration 000007 failed: runtime_artifacts rows without credential binding_id mapping'
            USING ERRCODE = '23514';
    END IF;

    SELECT COUNT(*) INTO duplicate_binding_runtime_artifacts
    FROM (
        SELECT cr.binding_id, ra.artifact_type, ra.scope_key
        FROM runtime_artifacts AS ra
        JOIN credential_records AS cr ON cr.platform_account_id = ra.platform_account_id
        GROUP BY cr.binding_id, ra.artifact_type, ra.scope_key
        HAVING COUNT(*) > 1
    ) AS duplicates;
    IF duplicate_binding_runtime_artifacts > 0 THEN
        RAISE EXCEPTION 'migration 000007 failed: duplicate runtime_artifacts rows for binding_id, artifact_type, scope_key'
            USING ERRCODE = '23514';
    END IF;
END
$$;

ALTER TABLE account_profiles
    ADD COLUMN default_profile_marker SMALLINT GENERATED ALWAYS AS (
        CASE WHEN is_default THEN 1 ELSE NULL END
    ) STORED;
CREATE UNIQUE INDEX uniq_default_profile_per_binding
    ON account_profiles (binding_id, default_profile_marker);

ALTER TABLE runtime_artifacts ADD COLUMN binding_id BIGINT;
CREATE INDEX idx_runtime_binding_id ON runtime_artifacts (binding_id);

UPDATE runtime_artifacts AS ra
SET binding_id = cr.binding_id
FROM credential_records AS cr
WHERE cr.platform_account_id = ra.platform_account_id;

DROP INDEX uniq_runtime_artifact;
ALTER TABLE runtime_artifacts ALTER COLUMN binding_id SET NOT NULL;
CREATE UNIQUE INDEX uniq_runtime_artifact_binding
    ON runtime_artifacts (binding_id, artifact_type, scope_key);
