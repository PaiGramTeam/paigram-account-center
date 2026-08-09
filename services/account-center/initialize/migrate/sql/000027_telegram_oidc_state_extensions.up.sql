ALTER TABLE user_oauth_states
    ADD COLUMN metadata JSON NULL COMMENT 'purpose-specific extension blob (e.g. {"bot_id":"..."})' AFTER user_agent,
    ADD COLUMN consumed_at DATETIME(3) NULL COMMENT 'NULL = available; non-NULL = redeemed' AFTER expires_at;
