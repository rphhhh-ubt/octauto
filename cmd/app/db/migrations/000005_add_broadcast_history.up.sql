CREATE TABLE IF NOT EXISTS broadcast_history
(
    id            SERIAL PRIMARY KEY,
    target_type   VARCHAR(50)  NOT NULL,
    message_text  TEXT         NOT NULL,
    total_count   INTEGER      DEFAULT 0,
    sent_count    INTEGER      DEFAULT 0,
    failed_count  INTEGER      DEFAULT 0,
    status        VARCHAR(20)  DEFAULT 'pending',
    created_at    TIMESTAMP    DEFAULT NOW(),
    completed_at  TIMESTAMP
);

CREATE INDEX idx_broadcast_status ON broadcast_history(status);
CREATE INDEX idx_broadcast_created_at ON broadcast_history(created_at DESC);
