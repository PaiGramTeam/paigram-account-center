-- PostgreSQL 1.0 release schema.

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
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
    CONSTRAINT uniq_provider_account UNIQUE (provider, provider_account_id),
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

CREATE TABLE bots (
    id VARCHAR(64) PRIMARY KEY,
    display_name VARCHAR(255) NOT NULL,
    description TEXT,
    type VARCHAR(32) NOT NULL DEFAULT 'OTHER',
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    owner_user_id BIGINT NOT NULL,
    allow_legacy_binding_write BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT fk_bots_owner FOREIGN KEY (owner_user_id) REFERENCES users (id) ON DELETE RESTRICT ON UPDATE CASCADE
);
CREATE INDEX idx_bots_owner ON bots (owner_user_id);
CREATE INDEX idx_bots_status ON bots (status);
CREATE INDEX idx_bots_deleted_at ON bots (deleted_at);

CREATE TABLE bot_identities (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    bot_id VARCHAR(64) NOT NULL,
    external_user_id VARCHAR(191) NOT NULL,
    external_username VARCHAR(255),
    linked_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uk_bot_identities_bot_external UNIQUE (bot_id, external_user_id),
    CONSTRAINT uk_bot_identities_user_bot UNIQUE (user_id, bot_id),
    CONSTRAINT fk_bot_identities_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_bot_identities_bot FOREIGN KEY (bot_id) REFERENCES bots (id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX idx_bot_identities_user_id ON bot_identities (user_id);
CREATE INDEX idx_bot_identities_deleted_at ON bot_identities (deleted_at);

CREATE TABLE service_credentials (
    client_id VARCHAR(96) PRIMARY KEY,
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

CREATE TABLE platform_services (
    id BIGSERIAL PRIMARY KEY,
    platform_key VARCHAR(64) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    service_key VARCHAR(128) NOT NULL,
    service_audience VARCHAR(128) NOT NULL,
    discovery_type VARCHAR(32) NOT NULL,
    endpoint VARCHAR(255) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    supported_actions_json JSONB NOT NULL,
    credential_schema_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uniq_platform_services_platform_key UNIQUE (platform_key),
    CONSTRAINT uniq_platform_services_service_key UNIQUE (service_key)
);

CREATE TABLE platform_account_refs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    platform VARCHAR(64) NOT NULL,
    platform_service_key VARCHAR(128) NOT NULL,
    platform_account_id VARCHAR(191) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    meta_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uk_platform_account_refs_platform_account UNIQUE (platform, platform_account_id),
    CONSTRAINT fk_platform_account_refs_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX idx_platform_account_refs_user_platform ON platform_account_refs (user_id, platform);
CREATE INDEX idx_platform_account_refs_status ON platform_account_refs (status);
CREATE INDEX idx_platform_account_refs_deleted_at ON platform_account_refs (deleted_at);

CREATE TABLE bot_account_grants (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    bot_id VARCHAR(64) NOT NULL,
    platform_account_ref_id BIGINT NOT NULL,
    scopes JSONB NOT NULL,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uk_bot_account_grants_bot_account UNIQUE (bot_id, platform_account_ref_id),
    CONSTRAINT fk_bot_account_grants_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_bot_account_grants_bot FOREIGN KEY (bot_id) REFERENCES bots (id) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_bot_account_grants_ref FOREIGN KEY (platform_account_ref_id) REFERENCES platform_account_refs (id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX idx_bot_account_grants_user_id ON bot_account_grants (user_id);
CREATE INDEX idx_bot_account_grants_platform_account_ref_id ON bot_account_grants (platform_account_ref_id);
CREATE INDEX idx_bot_account_grants_revoked_at ON bot_account_grants (revoked_at);
CREATE INDEX idx_bot_account_grants_deleted_at ON bot_account_grants (deleted_at);

CREATE TABLE platform_account_bindings (
    id BIGSERIAL PRIMARY KEY,
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    active_external_account_marker SMALLINT GENERATED ALWAYS AS (
        CASE WHEN deleted_at IS NULL AND external_account_key IS NOT NULL THEN 1 ELSE NULL END
    ) STORED,
    CONSTRAINT fk_platform_account_bindings_owner FOREIGN KEY (owner_user_id) REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE UNIQUE INDEX uk_platform_account_bindings_active_external_account
    ON platform_account_bindings (platform, external_account_key, active_external_account_marker);
CREATE INDEX idx_platform_account_bindings_owner ON platform_account_bindings (owner_user_id);
CREATE INDEX idx_platform_account_bindings_status ON platform_account_bindings (status);
CREATE INDEX idx_platform_account_bindings_primary_profile_id ON platform_account_bindings (primary_profile_id);
CREATE UNIQUE INDEX idx_platform_account_bindings_primary_profile_assignment ON platform_account_bindings (id, primary_profile_id);
CREATE INDEX idx_platform_account_bindings_deleted_at ON platform_account_bindings (deleted_at);

CREATE TABLE platform_account_profiles (
    id BIGSERIAL PRIMARY KEY,
    binding_id BIGINT NOT NULL,
    platform_profile_key VARCHAR(191) NOT NULL,
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
    CONSTRAINT uk_platform_account_profiles_binding_key UNIQUE (binding_id, platform_profile_key),
    CONSTRAINT uk_platform_account_profiles_binding_row UNIQUE (binding_id, id),
    CONSTRAINT fk_platform_account_profiles_binding FOREIGN KEY (binding_id) REFERENCES platform_account_bindings (id) ON DELETE CASCADE ON UPDATE CASCADE
);
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
    scopes_json TEXT NOT NULL,
    ticket_version BIGINT NOT NULL DEFAULT 1,
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
