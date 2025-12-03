-- Таблица промокодов на тариф
CREATE TABLE promo_tariff_code
(
    id                  BIGSERIAL PRIMARY KEY,
    code                VARCHAR(50) NOT NULL UNIQUE,
    price               INTEGER NOT NULL,           -- цена в рублях
    devices             INTEGER NOT NULL,           -- hwidDeviceLimit
    months              INTEGER NOT NULL,           -- период подписки
    max_activations     INTEGER NOT NULL,           -- лимит активаций
    current_activations INTEGER DEFAULT 0,
    valid_hours         INTEGER NOT NULL,           -- срок действия предложения после активации
    is_active           BOOLEAN DEFAULT TRUE,
    created_by_admin_id BIGINT NOT NULL,
    created_at          TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    valid_until         TIMESTAMP WITH TIME ZONE    -- дата истечения самого промокода (опционально)
);

CREATE INDEX idx_promo_tariff_code_code ON promo_tariff_code USING hash (code);
CREATE INDEX idx_promo_tariff_code_is_active ON promo_tariff_code (is_active);

-- Таблица активаций промокодов на тариф
CREATE TABLE promo_tariff_activation
(
    id              BIGSERIAL PRIMARY KEY,
    promo_tariff_id BIGINT NOT NULL REFERENCES promo_tariff_code (id) ON DELETE CASCADE,
    customer_id     BIGINT NOT NULL REFERENCES customer (id) ON DELETE CASCADE,
    activated_at    TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (promo_tariff_id, customer_id)
);

CREATE INDEX idx_promo_tariff_activation_customer ON promo_tariff_activation (customer_id);
CREATE INDEX idx_promo_tariff_activation_promo ON promo_tariff_activation (promo_tariff_id);

-- Поля promo offer в customer
ALTER TABLE customer ADD COLUMN promo_offer_price INTEGER;
ALTER TABLE customer ADD COLUMN promo_offer_devices INTEGER;
ALTER TABLE customer ADD COLUMN promo_offer_months INTEGER;
ALTER TABLE customer ADD COLUMN promo_offer_expires_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE customer ADD COLUMN promo_offer_code_id BIGINT REFERENCES promo_tariff_code(id);
