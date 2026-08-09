ALTER TABLE user_oauth_states
    ADD COLUMN metadata JSONB,
    ADD COLUMN consumed_at TIMESTAMPTZ;
