-- PostgreSQL 1.0 release schema.

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    user_ref VARCHAR(64) NOT NULL DEFAULT ('usr_' || gen_random_uuid()::text),
    owner_epoch BIGINT NOT NULL DEFAULT 1 CHECK (owner_epoch >= 1),
    primary_login_type VARCHAR(32) NOT NULL DEFAULT 'email',
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    primary_role_id BIGINT,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_users_status ON users (status);
CREATE INDEX idx_users_primary_role_id ON users (primary_role_id);
CREATE UNIQUE INDEX idx_users_primary_role_assignment ON users (id, primary_role_id);
CREATE UNIQUE INDEX uk_users_ref ON users (user_ref);
CREATE INDEX idx_users_last_login_at ON users (last_login_at);
CREATE INDEX idx_users_deleted_at ON users (deleted_at);

CREATE TABLE user_profiles (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL UNIQUE,
    display_name VARCHAR(255) NOT NULL,
    avatar_url VARCHAR(512),
    bio TEXT,
    locale VARCHAR(10) NOT NULL DEFAULT 'en_US',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_user_profiles_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE
);

CREATE TABLE user_credentials (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    provider VARCHAR(64) NOT NULL,
    issuer VARCHAR(255) NOT NULL,
    provider_account_id VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255),
    access_token TEXT,
    refresh_token TEXT,
    token_expiry TIMESTAMPTZ,
    scopes VARCHAR(512),
    last_sync_at TIMESTAMPTZ,
    metadata TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_user_credentials_issuer_nonempty CHECK (btrim(issuer) <> ''),
    CONSTRAINT uniq_issuer_subject UNIQUE (issuer, provider_account_id),
    CONSTRAINT uniq_user_provider UNIQUE (user_id, provider),
    CONSTRAINT fk_user_credentials_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX idx_user_credentials_provider_user ON user_credentials (provider, user_id);

CREATE TABLE user_emails (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    email VARCHAR(255) NOT NULL,
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    verified_at TIMESTAMPTZ,
    verification_token VARCHAR(255),
    verification_expiry TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uniq_email UNIQUE (email),
    CONSTRAINT fk_user_emails_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX idx_user_primary ON user_emails (user_id, is_primary);
CREATE INDEX idx_verification_token ON user_emails (verification_token);

CREATE TABLE user_oauth_states (
    id BIGSERIAL PRIMARY KEY,
    provider VARCHAR(64) NOT NULL,
    state VARCHAR(255) NOT NULL,
    purpose VARCHAR(64) NOT NULL DEFAULT 'login',
    user_id BIGINT,
    redirect_to VARCHAR(512),
    nonce VARCHAR(255),
    code_verifier VARCHAR(255),
    client_ip VARCHAR(64) NOT NULL DEFAULT '',
    user_agent VARCHAR(255) NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uniq_state UNIQUE (state),
    CONSTRAINT fk_user_oauth_states_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE SET NULL ON UPDATE CASCADE
);
CREATE INDEX idx_provider_expires ON user_oauth_states (provider, expires_at);
CREATE INDEX idx_oauth_state_code_verifier ON user_oauth_states (code_verifier);
CREATE INDEX idx_user_oauth_states_purpose ON user_oauth_states (purpose);
CREATE INDEX idx_user_oauth_states_user_id ON user_oauth_states (user_id);

CREATE TABLE user_sessions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    family_id VARCHAR(36) NOT NULL,
    access_token_hash VARCHAR(64) NOT NULL,
    refresh_token_hash VARCHAR(64) NOT NULL,
    access_expiry TIMESTAMPTZ NOT NULL,
    refresh_expiry TIMESTAMPTZ NOT NULL,
    user_agent VARCHAR(512),
    client_ip VARCHAR(128),
    revoked_at TIMESTAMPTZ,
    revoked_reason VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uniq_access_token_hash UNIQUE (access_token_hash),
    CONSTRAINT uniq_refresh_token_hash UNIQUE (refresh_token_hash),
    CONSTRAINT fk_user_sessions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX idx_user_sessions_user ON user_sessions (user_id);
CREATE INDEX idx_user_sessions_family ON user_sessions (family_id);
CREATE INDEX idx_refresh_expiry ON user_sessions (refresh_expiry);
CREATE INDEX idx_access_expiry ON user_sessions (access_expiry);

CREATE TABLE user_refresh_token_histories (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL,
    family_id VARCHAR(36) NOT NULL,
    token_hash VARCHAR(64) NOT NULL,
    used_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_user_refresh_token_histories_token_hash UNIQUE (token_hash),
    CONSTRAINT fk_user_refresh_token_histories_session FOREIGN KEY (session_id) REFERENCES user_sessions (id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX idx_user_refresh_token_histories_session ON user_refresh_token_histories (session_id);
CREATE INDEX idx_user_refresh_token_histories_family ON user_refresh_token_histories (family_id);
CREATE INDEX idx_user_refresh_token_histories_expiry ON user_refresh_token_histories (expires_at);

CREATE TABLE login_audits (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT,
    provider VARCHAR(64) NOT NULL,
    success BOOLEAN NOT NULL,
    client_ip VARCHAR(128),
    user_agent VARCHAR(512),
    message VARCHAR(512),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_login_audits_user ON login_audits (user_id);
CREATE INDEX idx_login_audits_provider ON login_audits (provider);

CREATE TABLE user_two_factors (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    secret VARCHAR(255) NOT NULL,
    backup_codes TEXT,
    enabled_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_user_id UNIQUE (user_id),
    CONSTRAINT fk_user_two_factor_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX idx_two_factor_enabled_at ON user_two_factors (enabled_at);

CREATE TABLE user_devices (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    device_id VARCHAR(255) NOT NULL,
    device_name VARCHAR(255),
    device_type VARCHAR(64),
    os VARCHAR(64),
    browser VARCHAR(64),
    ip VARCHAR(128),
    location VARCHAR(255),
    last_active_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    trust_expiry TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_device_id UNIQUE (device_id),
    CONSTRAINT fk_user_device_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX idx_user_devices ON user_devices (user_id);
CREATE INDEX idx_last_active ON user_devices (last_active_at);

CREATE TABLE login_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    login_type VARCHAR(32) NOT NULL,
    ip VARCHAR(128),
    user_agent VARCHAR(512),
    device VARCHAR(255),
    location VARCHAR(255),
    status VARCHAR(32) NOT NULL,
    failure_reason VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_login_log_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX idx_user_login_logs ON login_logs (user_id, created_at);
CREATE INDEX idx_login_type ON login_logs (login_type);
CREATE INDEX idx_login_logs_status ON login_logs (status);
CREATE INDEX idx_login_logs_created_at ON login_logs (created_at);

CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    action VARCHAR(128) NOT NULL,
    resource VARCHAR(128),
    resource_id BIGINT,
    old_value TEXT,
    new_value TEXT,
    ip VARCHAR(128),
    user_agent VARCHAR(512),
    details TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_audit_log_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX idx_user_audit_logs ON audit_logs (user_id, created_at);
CREATE INDEX idx_audit_logs_action ON audit_logs (action);
CREATE INDEX idx_audit_logs_resource ON audit_logs (resource_id);
CREATE INDEX idx_audit_logs_created_at ON audit_logs (created_at);

CREATE TABLE password_reset_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    token VARCHAR(255) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_token UNIQUE (token),
    CONSTRAINT fk_password_reset_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX idx_user_password_reset ON password_reset_tokens (user_id);
CREATE INDEX idx_password_reset_expires_at ON password_reset_tokens (expires_at);

CREATE TABLE permissions (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    resource VARCHAR(100) NOT NULL,
    action VARCHAR(50) NOT NULL,
    description VARCHAR(512),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_permissions_resource ON permissions (resource);
CREATE INDEX idx_permissions_action ON permissions (action);
CREATE INDEX idx_permissions_deleted_at ON permissions (deleted_at);

CREATE TABLE roles (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    display_name VARCHAR(255) NOT NULL,
    description VARCHAR(512),
    is_system BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_roles_is_system ON roles (is_system);
CREATE INDEX idx_roles_deleted_at ON roles (deleted_at);

CREATE TABLE role_permissions (
    role_id BIGINT NOT NULL,
    permission_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (role_id, permission_id),
    CONSTRAINT fk_role_permissions_role FOREIGN KEY (role_id) REFERENCES roles (id) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_role_permissions_permission FOREIGN KEY (permission_id) REFERENCES permissions (id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX idx_role_permission ON role_permissions (role_id, permission_id);

CREATE TABLE user_roles (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    role_id BIGINT NOT NULL,
    granted_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT idx_user_role UNIQUE (user_id, role_id),
    CONSTRAINT fk_user_roles_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_user_roles_role FOREIGN KEY (role_id) REFERENCES roles (id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX idx_user_roles_user_id ON user_roles (user_id);
CREATE INDEX idx_user_roles_role_id ON user_roles (role_id);
CREATE INDEX idx_user_roles_granted_by ON user_roles (granted_by);

ALTER TABLE users ADD CONSTRAINT fk_users_primary_role_assignment
    FOREIGN KEY (id, primary_role_id) REFERENCES user_roles (user_id, role_id)
    ON DELETE RESTRICT ON UPDATE CASCADE;

CREATE TABLE casbin_rule (
    id BIGSERIAL PRIMARY KEY,
    ptype VARCHAR(100),
    v0 VARCHAR(100),
    v1 VARCHAR(100),
    v2 VARCHAR(100),
    v3 VARCHAR(100),
    v4 VARCHAR(100),
    v5 VARCHAR(100),
    CONSTRAINT idx_casbin_rule UNIQUE NULLS NOT DISTINCT (ptype, v0, v1, v2, v3, v4, v5)
);

CREATE TABLE admin_guard (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE,
    armed BOOLEAN NOT NULL DEFAULT FALSE,
    revision BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ck_admin_guard_singleton CHECK (singleton)
);
INSERT INTO admin_guard (singleton) VALUES (TRUE);

CREATE FUNCTION recovery_administrator_exists() RETURNS BOOLEAN AS $$
    SELECT EXISTS (
        SELECT 1
        FROM users
        JOIN user_roles ON user_roles.user_id = users.id
        JOIN roles ON roles.id = user_roles.role_id
        WHERE users.status = 'active'
          AND users.deleted_at IS NULL
          AND roles.name = 'admin'
          AND roles.is_system = TRUE
          AND roles.deleted_at IS NULL
          AND EXISTS (
              SELECT 1
              FROM role_permissions
              JOIN permissions ON permissions.id = role_permissions.permission_id
              WHERE role_permissions.role_id = roles.id
                AND permissions.name = 'role:manage'
                AND permissions.deleted_at IS NULL
          )
          AND EXISTS (
              SELECT 1
              FROM casbin_rule
              WHERE casbin_rule.ptype = 'p'
                AND casbin_rule.v0 = roles.id::TEXT
                AND casbin_rule.v2 = 'PUT'
                AND casbin_rule.v1 IN ('/api/v1/admin/roles/:id/users', '/api/v1/*')
          )
          AND EXISTS (
              SELECT 1
              FROM casbin_rule
              WHERE casbin_rule.ptype = 'p'
                AND casbin_rule.v0 = roles.id::TEXT
                AND casbin_rule.v2 = 'PUT'
                AND casbin_rule.v1 IN ('/api/v1/admin/roles/:id/permissions', '/api/v1/*')
          )
    );
$$ LANGUAGE SQL STABLE;

CREATE FUNCTION validate_active_administrator_guard() RETURNS VOID AS $$
DECLARE
    guard_armed BOOLEAN;
    administrator_exists BOOLEAN;
BEGIN
    UPDATE admin_guard
    SET revision = revision + 1
    WHERE singleton = TRUE
    RETURNING armed INTO guard_armed;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            CONSTRAINT = 'ck_admin_guard_singleton',
            MESSAGE = 'administrator guard singleton is missing';
    END IF;
    SELECT recovery_administrator_exists() INTO administrator_exists;
    IF NOT guard_armed THEN
        IF administrator_exists THEN
            UPDATE admin_guard SET armed = TRUE WHERE singleton = TRUE;
        END IF;
        RETURN;
    END IF;
    IF NOT administrator_exists THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            CONSTRAINT = 'active_administrator_required',
            MESSAGE = 'at least one active administrator is required';
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION enforce_active_administrator_guard() RETURNS TRIGGER AS $$
BEGIN
    PERFORM validate_active_administrator_guard();
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION prevent_admin_guard_removal() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION USING
        ERRCODE = '23514',
        CONSTRAINT = 'ck_admin_guard_singleton',
        MESSAGE = 'administrator guard singleton cannot be removed';
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION prevent_admin_guard_downgrade() RETURNS TRIGGER AS $$
BEGIN
    IF OLD.armed AND NOT NEW.armed THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            CONSTRAINT = 'admin_guard_cannot_disarm',
            MESSAGE = 'administrator guard cannot be disarmed';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER active_administrator_guard_user_roles
    AFTER INSERT OR UPDATE OR DELETE ON user_roles
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION enforce_active_administrator_guard();

CREATE CONSTRAINT TRIGGER active_administrator_guard_users_update
    AFTER UPDATE ON users
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    WHEN (OLD.status IS DISTINCT FROM NEW.status OR OLD.deleted_at IS DISTINCT FROM NEW.deleted_at)
    EXECUTE FUNCTION enforce_active_administrator_guard();

CREATE CONSTRAINT TRIGGER active_administrator_guard_users_delete
    AFTER DELETE ON users
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION enforce_active_administrator_guard();

CREATE CONSTRAINT TRIGGER active_administrator_guard_roles_update
    AFTER UPDATE ON roles
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    WHEN (
        OLD.name IS DISTINCT FROM NEW.name
        OR OLD.is_system IS DISTINCT FROM NEW.is_system
        OR OLD.deleted_at IS DISTINCT FROM NEW.deleted_at
    )
    EXECUTE FUNCTION enforce_active_administrator_guard();

CREATE CONSTRAINT TRIGGER active_administrator_guard_roles_delete
    AFTER DELETE ON roles
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION enforce_active_administrator_guard();

CREATE CONSTRAINT TRIGGER active_administrator_guard_role_permissions
    AFTER INSERT OR UPDATE OR DELETE ON role_permissions
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION enforce_active_administrator_guard();

CREATE CONSTRAINT TRIGGER active_administrator_guard_permissions_update
    AFTER UPDATE ON permissions
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    WHEN (OLD.name IS DISTINCT FROM NEW.name OR OLD.deleted_at IS DISTINCT FROM NEW.deleted_at)
    EXECUTE FUNCTION enforce_active_administrator_guard();

CREATE CONSTRAINT TRIGGER active_administrator_guard_permissions_delete
    AFTER DELETE ON permissions
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION enforce_active_administrator_guard();

CREATE CONSTRAINT TRIGGER active_administrator_guard_casbin_rules
    AFTER INSERT OR UPDATE OR DELETE ON casbin_rule
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION enforce_active_administrator_guard();

CREATE TRIGGER admin_guard_prevent_delete
    BEFORE DELETE OR TRUNCATE ON admin_guard
    FOR EACH STATEMENT EXECUTE FUNCTION prevent_admin_guard_removal();

CREATE TRIGGER admin_guard_prevent_downgrade
    BEFORE UPDATE OF armed ON admin_guard
    FOR EACH ROW EXECUTE FUNCTION prevent_admin_guard_downgrade();

CREATE TABLE bots (
    id VARCHAR(64) PRIMARY KEY,
    entry_issuer VARCHAR(191) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    description TEXT,
    type VARCHAR(32) NOT NULL DEFAULT 'OTHER',
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    owner_user_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT fk_bots_owner FOREIGN KEY (owner_user_id) REFERENCES users (id) ON DELETE RESTRICT ON UPDATE CASCADE
);
CREATE INDEX idx_bots_owner ON bots (owner_user_id);
CREATE UNIQUE INDEX uk_bots_entry_issuer ON bots (entry_issuer);
CREATE INDEX idx_bots_status ON bots (status);
CREATE INDEX idx_bots_deleted_at ON bots (deleted_at);

CREATE TABLE bot_identities (
    id BIGSERIAL PRIMARY KEY,
    entry_identity_ref VARCHAR(64) NOT NULL DEFAULT ('entry_' || gen_random_uuid()::text),
    entry_epoch BIGINT NOT NULL DEFAULT 1 CHECK (entry_epoch >= 1),
    user_id BIGINT NOT NULL,
    bot_id VARCHAR(64) NOT NULL,
    issuer VARCHAR(191) NOT NULL,
    external_user_id VARCHAR(191) NOT NULL,
    external_username VARCHAR(255),
    linked_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT fk_bot_identities_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_bot_identities_bot FOREIGN KEY (bot_id) REFERENCES bots (id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX idx_bot_identities_user_id ON bot_identities (user_id);
CREATE UNIQUE INDEX uk_bot_identities_ref ON bot_identities (entry_identity_ref);
CREATE UNIQUE INDEX uk_bot_identities_issuer_subject_active ON bot_identities (issuer, external_user_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uk_bot_identities_user_issuer_active ON bot_identities (user_id, issuer) WHERE deleted_at IS NULL;
CREATE INDEX idx_bot_identities_deleted_at ON bot_identities (deleted_at);

CREATE TABLE bot_routes (
    id BIGSERIAL PRIMARY KEY,
    bot_id VARCHAR(64) NOT NULL,
    platform VARCHAR(32) NOT NULL,
    service_id VARCHAR(64) NOT NULL,
    endpoint VARCHAR(255) NOT NULL,
    handlers_json JSONB NOT NULL,
    version VARCHAR(32) NOT NULL,
    last_heartbeat_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_bot_routes_bot_platform UNIQUE (bot_id, platform)
);

CREATE TABLE bot_route_audit (
    id BIGSERIAL PRIMARY KEY,
    bot_id VARCHAR(64) NOT NULL,
    platform VARCHAR(32) NOT NULL,
    action VARCHAR(16) NOT NULL,
    payload JSONB NOT NULL,
    actor VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_bot_route_audit_bot ON bot_route_audit (bot_id, created_at);

CREATE TABLE service_credentials (
    client_id VARCHAR(96) PRIMARY KEY,
    consumer_epoch BIGINT NOT NULL DEFAULT 1 CHECK (consumer_epoch >= 1),
    bot_id VARCHAR(64) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    secret_hash VARCHAR(255) NOT NULL,
    audiences JSONB NOT NULL,
    scopes JSONB NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    owner_user_id BIGINT NOT NULL,
    description TEXT,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT fk_service_credentials_owner FOREIGN KEY (owner_user_id) REFERENCES users (id) ON DELETE RESTRICT ON UPDATE CASCADE,
    CONSTRAINT fk_service_credentials_bot FOREIGN KEY (bot_id) REFERENCES bots (id) ON DELETE RESTRICT ON UPDATE CASCADE
);
CREATE INDEX idx_service_credentials_status ON service_credentials (status);
CREATE INDEX idx_service_credentials_owner ON service_credentials (owner_user_id);
CREATE INDEX idx_service_credentials_bot ON service_credentials (bot_id);
CREATE INDEX idx_service_credentials_deleted_at ON service_credentials (deleted_at);

CREATE TABLE entry_identity_link_challenges (
    challenge_hash CHAR(64) PRIMARY KEY,
    consumer VARCHAR(96) NOT NULL,
    bot_id VARCHAR(64) NOT NULL,
    issuer VARCHAR(191) NOT NULL,
    external_subject VARCHAR(191) NOT NULL,
    external_username VARCHAR(255),
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    expires_at TIMESTAMPTZ NOT NULL,
    approved_user_id BIGINT,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_entry_identity_link_challenges_status CHECK (status IN ('pending', 'approved', 'cancelled', 'expired', 'conflict')),
    CONSTRAINT fk_entry_identity_link_challenges_consumer FOREIGN KEY (consumer) REFERENCES service_credentials (client_id) ON DELETE RESTRICT ON UPDATE CASCADE,
    CONSTRAINT fk_entry_identity_link_challenges_bot FOREIGN KEY (bot_id) REFERENCES bots (id) ON DELETE RESTRICT ON UPDATE CASCADE,
    CONSTRAINT fk_entry_identity_link_challenges_user FOREIGN KEY (approved_user_id) REFERENCES users (id) ON DELETE SET NULL ON UPDATE CASCADE
);
CREATE INDEX idx_entry_identity_link_challenges_consumer ON entry_identity_link_challenges (consumer);
CREATE INDEX idx_entry_identity_link_challenges_bot ON entry_identity_link_challenges (bot_id);
CREATE INDEX idx_entry_identity_link_challenges_status_expiry ON entry_identity_link_challenges (status, expires_at);
CREATE INDEX idx_entry_identity_link_challenges_user ON entry_identity_link_challenges (approved_user_id);

CREATE TABLE entry_identity_unlink_operations (
    operation_id VARCHAR(64) PRIMARY KEY,
    user_id BIGINT NOT NULL,
    bot_id VARCHAR(64) NOT NULL,
    entry_identity_ref VARCHAR(64) NOT NULL,
    minimum_entry_epoch BIGINT NOT NULL CHECK (minimum_entry_epoch >= 1),
    state VARCHAR(32) NOT NULL DEFAULT 'PROPAGATION_PENDING',
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_entry_identity_unlink_operations_state CHECK (state IN ('PROPAGATION_PENDING', 'UNLINKED')),
    CONSTRAINT fk_entry_identity_unlink_operations_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_entry_identity_unlink_operations_bot FOREIGN KEY (bot_id) REFERENCES bots (id) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_entry_identity_unlink_operations_identity FOREIGN KEY (entry_identity_ref) REFERENCES bot_identities (entry_identity_ref) ON DELETE RESTRICT ON UPDATE CASCADE
);
CREATE INDEX idx_entry_identity_unlink_operations_user_bot ON entry_identity_unlink_operations (user_id, bot_id);
CREATE INDEX idx_entry_identity_unlink_operations_state ON entry_identity_unlink_operations (state);

CREATE TABLE platform_services (
    id BIGSERIAL PRIMARY KEY,
    platform_key VARCHAR(64) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    service_key VARCHAR(128) NOT NULL,
    service_audience VARCHAR(128) NOT NULL,
    discovery_type VARCHAR(32) NOT NULL,
    control_endpoint VARCHAR(255) NOT NULL,
    runtime_endpoint VARCHAR(255) NOT NULL,
    runtime_server_name VARCHAR(255) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    supported_actions_json JSONB NOT NULL,
    credential_schema_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uniq_platform_services_platform_key UNIQUE (platform_key),
    CONSTRAINT uniq_platform_services_service_key UNIQUE (service_key)
);

CREATE TABLE platform_account_bindings (
    id BIGSERIAL PRIMARY KEY,
    binding_ref VARCHAR(64) NOT NULL,
    generation BIGINT NOT NULL DEFAULT 0,
    owner_user_id BIGINT NOT NULL,
    platform VARCHAR(64) NOT NULL,
    external_account_key VARCHAR(191),
    platform_service_key VARCHAR(128) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending_bind',
    status_reason_code VARCHAR(64),
    status_reason_message VARCHAR(255),
    primary_profile_id BIGINT,
    last_validated_at TIMESTAMPTZ,
    last_synced_at TIMESTAMPTZ,
    profile_revision BIGINT NOT NULL DEFAULT 0,
    profile_observed_revision BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    active_external_account_marker SMALLINT GENERATED ALWAYS AS (
        CASE WHEN deleted_at IS NULL AND external_account_key IS NOT NULL THEN 1 ELSE NULL END
    ) STORED,
    CONSTRAINT chk_platform_account_bindings_ref_nonempty CHECK (binding_ref <> ''),
    CONSTRAINT chk_platform_account_bindings_generation_nonnegative CHECK (generation >= 0),
    CONSTRAINT chk_platform_account_bindings_profile_observation CHECK (profile_observed_revision <= profile_revision),
    CONSTRAINT uk_platform_account_bindings_identity UNIQUE (id, owner_user_id, platform),
    CONSTRAINT fk_platform_account_bindings_owner FOREIGN KEY (owner_user_id) REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE UNIQUE INDEX uk_platform_account_bindings_ref ON platform_account_bindings (binding_ref);
CREATE UNIQUE INDEX uk_platform_account_bindings_active_external_account
    ON platform_account_bindings (platform, external_account_key, active_external_account_marker);
CREATE INDEX idx_platform_account_bindings_owner ON platform_account_bindings (owner_user_id);
CREATE INDEX idx_platform_account_bindings_status ON platform_account_bindings (status);
CREATE INDEX idx_platform_account_bindings_primary_profile_id ON platform_account_bindings (primary_profile_id);
CREATE UNIQUE INDEX idx_platform_account_bindings_primary_profile_assignment ON platform_account_bindings (id, primary_profile_id);
CREATE INDEX idx_platform_account_bindings_deleted_at ON platform_account_bindings (deleted_at);

CREATE TABLE platform_operation_intents (
    operation_id VARCHAR(64) PRIMARY KEY,
    binding_id BIGINT NOT NULL,
    binding_ref VARCHAR(64) NOT NULL,
    owner_user_id BIGINT NOT NULL,
    platform VARCHAR(64) NOT NULL,
    kind VARCHAR(64) NOT NULL,
    pre_generation BIGINT NOT NULL,
    target_generation BIGINT NOT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    profile_ref VARCHAR(64) NOT NULL DEFAULT '',
    profile_revision BIGINT NOT NULL DEFAULT 0,
    delivery_mode VARCHAR(32) NOT NULL,
    state VARCHAR(32) NOT NULL,
    reason_code VARCHAR(64),
    actor_type VARCHAR(32) NOT NULL,
    actor_id VARCHAR(191) NOT NULL,
    input_expires_at TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    active_reservation_marker SMALLINT GENERATED ALWAYS AS (
        CASE WHEN state IN ('pending_delivery', 'uncertain', 'projection_pending', 'input_required', 'invariant_violation') THEN 1 ELSE NULL END
    ) STORED,
    bind_reservation_marker SMALLINT GENERATED ALWAYS AS (
        CASE WHEN kind = 'OPERATION_KIND_BIND_CREDENTIAL' AND state IN ('pending_delivery', 'uncertain', 'projection_pending', 'input_required', 'invariant_violation') THEN 1 ELSE NULL END
    ) STORED,
    CONSTRAINT chk_platform_operation_intents_generation CHECK (
        pre_generation >= 0 AND (
            (kind = 'OPERATION_KIND_SET_PRIMARY_PROFILE' AND target_generation = pre_generation)
            OR (kind <> 'OPERATION_KIND_SET_PRIMARY_PROFILE' AND target_generation = pre_generation + 1)
        )
    ),
    CONSTRAINT chk_platform_operation_intents_kind CHECK (
        kind IN (
            'OPERATION_KIND_BIND_CREDENTIAL',
            'OPERATION_KIND_REPLACE_CREDENTIAL',
            'OPERATION_KIND_REFRESH_CREDENTIAL',
            'OPERATION_KIND_DELETE_CREDENTIAL',
            'OPERATION_KIND_SET_PRIMARY_PROFILE'
        )
    ),
    CONSTRAINT chk_platform_operation_intents_profile CHECK (
        (kind = 'OPERATION_KIND_SET_PRIMARY_PROFILE' AND profile_ref <> '' AND profile_revision > 0)
        OR (kind <> 'OPERATION_KIND_SET_PRIMARY_PROFILE' AND profile_ref = '' AND profile_revision = 0)
    ),
    CONSTRAINT chk_platform_operation_intents_state CHECK (
        state IN ('pending_delivery', 'uncertain', 'projection_pending', 'input_required', 'invariant_violation', 'succeeded', 'failed', 'superseded')
    ),
    CONSTRAINT chk_platform_operation_intents_delivery_mode CHECK (delivery_mode IN ('sync_secret', 'outbox')),
    CONSTRAINT fk_platform_operation_intents_binding FOREIGN KEY (binding_id) REFERENCES platform_account_bindings (id) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_platform_operation_intents_binding_identity FOREIGN KEY (binding_id, owner_user_id, platform) REFERENCES platform_account_bindings (id, owner_user_id, platform) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_platform_operation_intents_owner FOREIGN KEY (owner_user_id) REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE UNIQUE INDEX uk_platform_operation_intents_active_binding
    ON platform_operation_intents (binding_id, active_reservation_marker);
CREATE UNIQUE INDEX uk_platform_operation_intents_active_owner_platform_bind
    ON platform_operation_intents (owner_user_id, platform, bind_reservation_marker);
CREATE INDEX idx_platform_operation_intents_binding_id ON platform_operation_intents (binding_id);
CREATE INDEX idx_platform_operation_intents_owner ON platform_operation_intents (owner_user_id);
CREATE INDEX idx_platform_operation_intents_state ON platform_operation_intents (state);

CREATE TABLE platform_operation_outbox (
    id BIGSERIAL PRIMARY KEY,
    operation_id VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    available_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_reason_code VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_platform_operation_outbox_status CHECK (status IN ('pending', 'completed', 'dead_letter')),
    CONSTRAINT uk_platform_operation_outbox_operation UNIQUE (operation_id),
    CONSTRAINT fk_platform_operation_outbox_intent FOREIGN KEY (operation_id) REFERENCES platform_operation_intents (operation_id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX idx_platform_operation_outbox_due ON platform_operation_outbox (status, available_at);

CREATE TABLE platform_account_profiles (
    id BIGSERIAL PRIMARY KEY,
    binding_id BIGINT NOT NULL,
    platform_profile_key VARCHAR(191) NOT NULL,
    profile_ref VARCHAR(64) NOT NULL,
    game_biz VARCHAR(64) NOT NULL,
    region VARCHAR(64) NOT NULL,
    player_uid VARCHAR(64) NOT NULL,
    nickname VARCHAR(255) NOT NULL,
    level BIGINT,
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    source_updated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    primary_profile_marker SMALLINT GENERATED ALWAYS AS (CASE WHEN is_primary THEN 1 ELSE NULL END) STORED,
    CONSTRAINT chk_platform_account_profiles_ref_nonempty CHECK (profile_ref <> ''),
    CONSTRAINT uk_platform_account_profiles_binding_key UNIQUE (binding_id, platform_profile_key),
    CONSTRAINT uk_platform_account_profiles_binding_row UNIQUE (binding_id, id),
    CONSTRAINT fk_platform_account_profiles_binding FOREIGN KEY (binding_id) REFERENCES platform_account_bindings (id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE UNIQUE INDEX uk_platform_account_profiles_ref ON platform_account_profiles (profile_ref);
CREATE UNIQUE INDEX uk_platform_account_profiles_primary_per_binding
    ON platform_account_profiles (binding_id, primary_profile_marker);
CREATE INDEX idx_platform_account_profiles_binding_id ON platform_account_profiles (binding_id);
CREATE INDEX idx_platform_account_profiles_player_uid ON platform_account_profiles (player_uid);

ALTER TABLE platform_account_bindings ADD CONSTRAINT fk_platform_account_bindings_primary_profile
    FOREIGN KEY (id, primary_profile_id) REFERENCES platform_account_profiles (binding_id, id)
    ON DELETE RESTRICT ON UPDATE CASCADE;

CREATE TABLE consumer_grants (
    id BIGSERIAL PRIMARY KEY,
    binding_id BIGINT NOT NULL,
    consumer VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    ticket_version BIGINT NOT NULL DEFAULT 1,
    pending_entry_epoch BIGINT NOT NULL DEFAULT 0 CHECK (pending_entry_epoch >= 0),
    granted_by BIGINT,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at TIMESTAMPTZ,
    last_invalidated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_consumer_grants_binding_consumer UNIQUE (binding_id, consumer),
    CONSTRAINT fk_consumer_grants_binding FOREIGN KEY (binding_id) REFERENCES platform_account_bindings (id) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_consumer_grants_granted_by FOREIGN KEY (granted_by) REFERENCES users (id) ON DELETE SET NULL ON UPDATE CASCADE
);
CREATE INDEX idx_consumer_grants_binding_id ON consumer_grants (binding_id);
CREATE INDEX idx_consumer_grants_status ON consumer_grants (status);
CREATE INDEX idx_consumer_grants_granted_by ON consumer_grants (granted_by);

CREATE TABLE consumer_grant_actions (
    grant_id BIGINT NOT NULL,
    action VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (grant_id, action),
    CONSTRAINT fk_consumer_grant_actions_grant FOREIGN KEY (grant_id) REFERENCES consumer_grants (id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX idx_consumer_grant_actions_action ON consumer_grant_actions (action);

CREATE TABLE entry_identity_unlink_targets (
    operation_id VARCHAR(64) NOT NULL,
    grant_id BIGINT NOT NULL,
    confirmed_at TIMESTAMPTZ,
    PRIMARY KEY (operation_id, grant_id),
    CONSTRAINT fk_entry_identity_unlink_targets_operation FOREIGN KEY (operation_id) REFERENCES entry_identity_unlink_operations (operation_id) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_entry_identity_unlink_targets_grant FOREIGN KEY (grant_id) REFERENCES consumer_grants (id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX idx_entry_identity_unlink_targets_grant ON entry_identity_unlink_targets (grant_id);

CREATE FUNCTION validate_consumer_grant_actions(target_grant_id BIGINT) RETURNS VOID AS $$
DECLARE
    grant_status VARCHAR(32);
BEGIN
    SELECT status INTO grant_status
    FROM consumer_grants
    WHERE id = target_grant_id
    FOR UPDATE;
    IF grant_status = 'active' AND NOT EXISTS (
        SELECT 1 FROM consumer_grant_actions WHERE grant_id = target_grant_id
    ) THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            MESSAGE = 'active consumer grant requires at least one action';
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION enforce_consumer_grant_actions_from_grant() RETURNS TRIGGER AS $$
BEGIN
    PERFORM validate_consumer_grant_actions(NEW.id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION enforce_consumer_grant_actions_from_action() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM validate_consumer_grant_actions(OLD.grant_id);
        RETURN OLD;
    END IF;
    IF TG_OP = 'UPDATE' AND OLD.grant_id <> NEW.grant_id THEN
        PERFORM validate_consumer_grant_actions(OLD.grant_id);
    END IF;
    PERFORM validate_consumer_grant_actions(NEW.grant_id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER consumer_grants_require_actions
    AFTER INSERT OR UPDATE ON consumer_grants
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION enforce_consumer_grant_actions_from_grant();

CREATE CONSTRAINT TRIGGER consumer_grant_actions_preserve_active_grant
    AFTER INSERT OR UPDATE OR DELETE ON consumer_grant_actions
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION enforce_consumer_grant_actions_from_action();

CREATE TABLE system_config_entries (
    id BIGSERIAL PRIMARY KEY,
    config_domain VARCHAR(64) NOT NULL,
    payload_json JSONB NOT NULL,
    version BIGINT NOT NULL DEFAULT 1,
    updated_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uk_system_config_entries_domain UNIQUE (config_domain),
    CONSTRAINT fk_system_config_entries_updated_by FOREIGN KEY (updated_by) REFERENCES users (id) ON DELETE SET NULL
);
CREATE INDEX idx_system_config_entries_updated_by ON system_config_entries (updated_by);
CREATE INDEX idx_system_config_entries_deleted_at ON system_config_entries (deleted_at);

CREATE TABLE legal_documents (
    id BIGSERIAL PRIMARY KEY,
    document_type VARCHAR(32) NOT NULL,
    version VARCHAR(64) NOT NULL,
    title VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    published BOOLEAN NOT NULL DEFAULT FALSE,
    published_at TIMESTAMPTZ,
    updated_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uk_legal_document_type_version UNIQUE (document_type, version),
    CONSTRAINT fk_legal_documents_updated_by FOREIGN KEY (updated_by) REFERENCES users (id) ON DELETE SET NULL
);
CREATE INDEX idx_legal_documents_published ON legal_documents (published);
CREATE INDEX idx_legal_documents_updated_by ON legal_documents (updated_by);
CREATE INDEX idx_legal_documents_deleted_at ON legal_documents (deleted_at);

CREATE TABLE audit_events (
    id BIGSERIAL PRIMARY KEY,
    category VARCHAR(64) NOT NULL,
    actor_type VARCHAR(32) NOT NULL,
    actor_user_id BIGINT,
    action VARCHAR(128) NOT NULL,
    target_type VARCHAR(64),
    target_id VARCHAR(191),
    binding_id BIGINT,
    result VARCHAR(32) NOT NULL,
    reason_code VARCHAR(64),
    request_id VARCHAR(128),
    ip VARCHAR(128),
    user_agent VARCHAR(512),
    metadata_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_audit_events_actor_user FOREIGN KEY (actor_user_id) REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT fk_audit_events_binding FOREIGN KEY (binding_id) REFERENCES platform_account_bindings (id) ON DELETE SET NULL
);
CREATE INDEX idx_audit_events_category ON audit_events (category);
CREATE INDEX idx_audit_events_actor_type ON audit_events (actor_type);
CREATE INDEX idx_audit_events_actor_user_id ON audit_events (actor_user_id);
CREATE INDEX idx_audit_events_action ON audit_events (action);
CREATE INDEX idx_audit_events_target_type ON audit_events (target_type);
CREATE INDEX idx_audit_events_target_id ON audit_events (target_id);
CREATE INDEX idx_audit_events_binding_id ON audit_events (binding_id);
CREATE INDEX idx_audit_events_result ON audit_events (result);
CREATE INDEX idx_audit_events_reason_code ON audit_events (reason_code);
CREATE INDEX idx_audit_events_request_id ON audit_events (request_id);
CREATE INDEX idx_audit_events_created_at ON audit_events (created_at);
