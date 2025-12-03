CREATE TABLE promo_code
(
    id                  BIGSERIAL PRIMARY KEY,
    code                VARCHAR(50) NOT NULL UNIQUE,
    bonus_days          INTEGER NOT NULL,
    max_activations     INTEGER NOT NULL,
    current_activations INTEGER DEFAULT 0,
    is_active           BOOLEAN DEFAULT TRUE,
    created_by_admin_id BIGINT NOT NULL,
    created_at          TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    valid_until         TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_promo_code_code ON promo_code USING hash (code);
CREATE INDEX idx_promo_code_is_active ON promo_code (is_active);

CREATE TABLE promo_code_activation
(
    id            BIGSERIAL PRIMARY KEY,
    promo_code_id BIGINT NOT NULL REFERENCES promo_code (id) ON DELETE CASCADE,
    customer_id   BIGINT NOT NULL REFERENCES customer (id) ON DELETE CASCADE,
    activated_at  TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (promo_code_id, customer_id)
);

CREATE INDEX idx_promo_activation_customer ON promo_code_activation (customer_id);
CREATE INDEX idx_promo_activation_promo ON promo_code_activation (promo_code_id);
