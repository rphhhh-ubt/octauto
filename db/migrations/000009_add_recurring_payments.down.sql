-- Откат миграции: удаляем поля рекуррентных платежей

ALTER TABLE customer DROP COLUMN IF EXISTS recurring_enabled;
ALTER TABLE customer DROP COLUMN IF EXISTS payment_method_id;
ALTER TABLE customer DROP COLUMN IF EXISTS recurring_tariff_name;
ALTER TABLE customer DROP COLUMN IF EXISTS recurring_months;
ALTER TABLE customer DROP COLUMN IF EXISTS recurring_amount;
ALTER TABLE customer DROP COLUMN IF EXISTS recurring_notified_at;
