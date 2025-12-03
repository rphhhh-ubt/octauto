-- Добавляем поля для отслеживания уведомлений триальных пользователей

-- Уведомление о неактивности триала
ALTER TABLE customer ADD COLUMN trial_inactive_notified_at TIMESTAMP WITH TIME ZONE;

-- Winback предложение
ALTER TABLE customer ADD COLUMN winback_offer_sent_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE customer ADD COLUMN winback_offer_expires_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE customer ADD COLUMN winback_offer_price INTEGER;
ALTER TABLE customer ADD COLUMN winback_offer_devices INTEGER;
ALTER TABLE customer ADD COLUMN winback_offer_months INTEGER;
