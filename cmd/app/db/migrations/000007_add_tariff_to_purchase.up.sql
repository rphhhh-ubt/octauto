-- Добавляем поле tariff_name в таблицу purchase для хранения выбранного тарифа
ALTER TABLE purchase ADD COLUMN tariff_name VARCHAR(50);

-- Добавляем поле device_limit для хранения лимита устройств из winback/promo предложений
ALTER TABLE purchase ADD COLUMN device_limit INTEGER;
