-- Добавляем поля для рекуррентных платежей

-- Флаг включения автопродления
ALTER TABLE customer ADD COLUMN recurring_enabled BOOLEAN DEFAULT FALSE;

-- ID сохранённого способа оплаты в ЮKassa
ALTER TABLE customer ADD COLUMN payment_method_id UUID;

-- Настройки автопродления (тариф, период, сумма)
ALTER TABLE customer ADD COLUMN recurring_tariff_name VARCHAR(100);
ALTER TABLE customer ADD COLUMN recurring_months INTEGER DEFAULT 1;
ALTER TABLE customer ADD COLUMN recurring_amount INTEGER;

-- Время последнего уведомления о предстоящем списании
ALTER TABLE customer ADD COLUMN recurring_notified_at TIMESTAMP WITH TIME ZONE;
