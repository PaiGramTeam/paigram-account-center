-- Bot route registry: 1:1:1 mapping (bot_id, platform) -> active game-service.
-- Telegram dispatch reads this table to resolve "(bot, platform) => endpoint".
CREATE TABLE IF NOT EXISTS bot_routes (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    bot_id            VARCHAR(64)  NOT NULL,
    platform          VARCHAR(32)  NOT NULL,
    service_id        VARCHAR(64)  NOT NULL,
    endpoint          VARCHAR(255) NOT NULL,
    handlers_json     JSON         NOT NULL,
    version           VARCHAR(32)  NOT NULL,
    last_heartbeat_at DATETIME(3)  NULL,
    created_at        DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at        DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_bot_routes_bot_platform (bot_id, platform)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Bot platform routing table';

-- Audit log for bot route registrations / unregistrations. Append-only.
CREATE TABLE IF NOT EXISTS bot_route_audit (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    bot_id     VARCHAR(64)  NOT NULL,
    platform   VARCHAR(32)  NOT NULL,
    action     VARCHAR(16)  NOT NULL,
    payload    JSON         NOT NULL,
    actor      VARCHAR(64)  NOT NULL,
    created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    KEY idx_bot_route_audit_bot (bot_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Bot route lifecycle audit log';
