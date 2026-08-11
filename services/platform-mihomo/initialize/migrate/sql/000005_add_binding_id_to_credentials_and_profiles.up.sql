DO $$
DECLARE
    invalid_credential_account_ids BIGINT;
    duplicate_parsed_credential_binding_ids BIGINT;
    invalid_profile_account_ids BIGINT;
BEGIN
    SELECT COUNT(*) INTO invalid_credential_account_ids
    FROM credential_records
    WHERE platform_account_id IS NULL
       OR NOT (
           platform_account_id ~ '^binding_[1-9][0-9]*_.+$'
           OR platform_account_id ~ '^hoyo_ref_[1-9][0-9]*_.+$'
       );
    IF invalid_credential_account_ids > 0 THEN
        RAISE EXCEPTION 'migration 000005 failed: malformed credential_records platform_account_id values'
            USING ERRCODE = '23514';
    END IF;

    SELECT COUNT(*) INTO duplicate_parsed_credential_binding_ids
    FROM (
        SELECT CASE
            WHEN platform_account_id ~ '^binding_[1-9][0-9]*_.+$' THEN split_part(platform_account_id, '_', 2)::BIGINT
            WHEN platform_account_id ~ '^hoyo_ref_[1-9][0-9]*_.+$' THEN split_part(platform_account_id, '_', 3)::BIGINT
        END AS parsed_binding_id
        FROM credential_records
        WHERE platform_account_id ~ '^binding_[1-9][0-9]*_.+$'
           OR platform_account_id ~ '^hoyo_ref_[1-9][0-9]*_.+$'
        GROUP BY parsed_binding_id
        HAVING COUNT(*) > 1
    ) AS duplicate_credential_binding_ids;
    IF duplicate_parsed_credential_binding_ids > 0 THEN
        RAISE EXCEPTION 'migration 000005 failed: duplicate parsed credential_records binding_id values'
            USING ERRCODE = '23514';
    END IF;

    SELECT COUNT(*) INTO invalid_profile_account_ids
    FROM account_profiles
    WHERE platform_account_id IS NULL
       OR NOT (
           platform_account_id ~ '^binding_[1-9][0-9]*_.+$'
           OR platform_account_id ~ '^hoyo_ref_[1-9][0-9]*_.+$'
       );
    IF invalid_profile_account_ids > 0 THEN
        RAISE EXCEPTION 'migration 000005 failed: malformed account_profiles platform_account_id values'
            USING ERRCODE = '23514';
    END IF;
END
$$;

ALTER TABLE credential_records ADD COLUMN binding_id BIGINT;

UPDATE credential_records
SET binding_id = CASE
    WHEN platform_account_id ~ '^binding_[1-9][0-9]*_.+$' THEN split_part(platform_account_id, '_', 2)::BIGINT
    WHEN platform_account_id ~ '^hoyo_ref_[1-9][0-9]*_.+$' THEN split_part(platform_account_id, '_', 3)::BIGINT
END
WHERE binding_id IS NULL;

ALTER TABLE credential_records ALTER COLUMN binding_id SET NOT NULL;
CREATE UNIQUE INDEX uniq_credential_binding_id ON credential_records (binding_id);

ALTER TABLE account_profiles ADD COLUMN binding_id BIGINT;

UPDATE account_profiles
SET binding_id = CASE
    WHEN platform_account_id ~ '^binding_[1-9][0-9]*_.+$' THEN split_part(platform_account_id, '_', 2)::BIGINT
    WHEN platform_account_id ~ '^hoyo_ref_[1-9][0-9]*_.+$' THEN split_part(platform_account_id, '_', 3)::BIGINT
END
WHERE binding_id IS NULL;

ALTER TABLE account_profiles ALTER COLUMN binding_id SET NOT NULL;
CREATE INDEX idx_profile_binding_id ON account_profiles (binding_id);
