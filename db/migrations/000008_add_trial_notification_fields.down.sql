-- Откат миграции: удаляем поля уведомлений

ALTER TABLE customer DROP COLUMN IF EXISTS trial_inactive_notified_at;
ALTER TABLE customer DROP COLUMN IF EXISTS winback_offer_sent_at;
ALTER TABLE customer DROP COLUMN IF EXISTS winback_offer_expires_at;
ALTER TABLE customer DROP COLUMN IF EXISTS winback_offer_price;
ALTER TABLE customer DROP COLUMN IF EXISTS winback_offer_devices;
ALTER TABLE customer DROP COLUMN IF EXISTS winback_offer_months;
