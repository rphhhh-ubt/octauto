package handler

import "log/slog"

const (
	CallbackBuy                 = "buy"
	CallbackSell                = "sell"
	CallbackStart               = "start"
	CallbackConnect             = "connect"
	CallbackPayment             = "payment"
	CallbackTrial               = "trial"
	CallbackActivateTrial       = "activate_trial"
	CallbackReferral            = "referral"
	CallbackPromo               = "promo"
	CallbackTariff              = "tariff"
	CallbackWinbackActivate     = "winback_activate"
	CallbackRecurringToggle        = "recurring_toggle"
	CallbackRecurringDisable       = "recurring_disable"
	CallbackDeletePaymentMethod    = "delete_payment_method"
	CallbackSavedPaymentMethods    = "saved_payment_methods"
	CallbackPromoTariff            = "promo_tariff"
)

// MaxCallbackDataLength - максимальная длина callback_data в Telegram (64 байта)
const MaxCallbackDataLength = 64

// SafeCallbackData проверяет длину callback_data и логирует warning если близко к лимиту
// Telegram обрезает callback_data > 64 байт, что может привести к ошибкам парсинга
func SafeCallbackData(data string) string {
	if len(data) > MaxCallbackDataLength {
		slog.Error("Callback data exceeds Telegram limit, will be truncated",
			"length", len(data),
			"maxLength", MaxCallbackDataLength,
			"data", data)
	} else if len(data) > 55 {
		slog.Warn("Callback data is close to Telegram limit",
			"length", len(data),
			"maxLength", MaxCallbackDataLength,
			"data", data)
	}
	return data
}
