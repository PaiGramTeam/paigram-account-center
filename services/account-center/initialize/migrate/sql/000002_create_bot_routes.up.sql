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
