-- Удаляем поля tariff_name и device_limit из таблицы purchase
ALTER TABLE purchase DROP COLUMN IF EXISTS device_limit;
ALTER TABLE purchase DROP COLUMN IF EXISTS tariff_name;
