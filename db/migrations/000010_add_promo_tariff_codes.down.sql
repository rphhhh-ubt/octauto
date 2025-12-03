-- Удаляем поля promo offer из customer
ALTER TABLE customer DROP COLUMN IF EXISTS promo_offer_code_id;
ALTER TABLE customer DROP COLUMN IF EXISTS promo_offer_expires_at;
ALTER TABLE customer DROP COLUMN IF EXISTS promo_offer_months;
ALTER TABLE customer DROP COLUMN IF EXISTS promo_offer_devices;
ALTER TABLE customer DROP COLUMN IF EXISTS promo_offer_price;

-- Удаляем таблицу активаций
DROP TABLE IF EXISTS promo_tariff_activation;

-- Удаляем таблицу промокодов на тариф
DROP TABLE IF EXISTS promo_tariff_code;
