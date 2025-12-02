package config

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

// Tariff представляет тарифный план с лимитом устройств и ценами
type Tariff struct {
	Name         string // Имя тарифа (START, PRO, etc.)
	Devices      int    // Лимит устройств (hwidDeviceLimit)
	Price1       int    // Цена за 1 месяц (рубли)
	Price3       int    // Цена за 3 месяца
	Price6       int    // Цена за 6 месяцев
	Price12      int    // Цена за 12 месяцев
	StarsPrice1  int    // Цена за 1 месяц (звёзды)
	StarsPrice3  int    // Цена за 3 месяца (звёзды)
	StarsPrice6  int    // Цена за 6 месяцев (звёзды)
	StarsPrice12 int    // Цена за 12 месяцев (звёзды)
	TributeURL   string // URL для оплаты через Tribute (опционально)
	TributeName  string // Название подписки в Tribute для матчинга webhook (опционально)
}

// Price возвращает цену тарифа за указанное количество месяцев
func (t Tariff) Price(month int) int {
	switch month {
	case 1:
		return t.Price1
	case 3:
		return t.Price3
	case 6:
		return t.Price6
	case 12:
		return t.Price12
	default:
		return t.Price1
	}
}

// StarsPrice возвращает цену в звёздах за указанное количество месяцев
func (t Tariff) StarsPrice(month int) int {
	switch month {
	case 1:
		return t.StarsPrice1
	case 3:
		return t.StarsPrice3
	case 6:
		return t.StarsPrice6
	case 12:
		return t.StarsPrice12
	default:
		return t.StarsPrice1
	}
}

// FormatButtonText форматирует текст кнопки тарифа
// Формат: "📱 {Name} — {Devices} устр."
func (t Tariff) FormatButtonText() string {
	return fmt.Sprintf("📱 %s — %d устр.", t.Name, t.Devices)
}

type config struct {
	telegramToken                                             string
	price1, price3, price6, price12                           int
	starsPrice1, starsPrice3, starsPrice6, starsPrice12       int
	remnawaveUrl, remnawaveToken, remnawaveMode, remnawaveTag string
	defaultLanguage                                           string
	databaseURL                                               string
	cryptoPayURL, cryptoPayToken                              string
	botURL                                                    string
	yookasaURL, yookasaShopId, yookasaSecretKey, yookasaEmail string
	trafficLimit, trialTrafficLimit                           int
	feedbackURL                                               string
	channelURL                                                string
	serverStatusURL                                           string
	supportURL                                                string
	tosURL                                                    string
	isYookasaEnabled                                          bool
	isCryptoEnabled                                           bool
	isTelegramStarsEnabled                                    bool
	adminTelegramId                                           int64
	trialDays                                                 int
	trialRemnawaveTag                                         string
	squadUUIDs                                                map[uuid.UUID]uuid.UUID
	referralDays                                              int
	miniApp                                                   string
	enableAutoPayment                                         bool
	healthCheckPort                                           int
	tributeWebhookUrl, tributeAPIKey, tributePaymentUrl       string
	isWebAppLinkEnabled                                       bool
	webhookEnabled                                            bool
	webhookURL                                                string
	webhookSecretToken                                        string
	daysInMonth                                               int
	externalSquadUUID                                         uuid.UUID
	blockedTelegramIds                                        map[int64]bool
	whitelistedTelegramIds                                    map[int64]bool
	requirePaidPurchaseForStars                               bool
	trialInternalSquads                                       map[uuid.UUID]uuid.UUID
	trialExternalSquadUUID                                    uuid.UUID
	remnawaveHeaders                                          map[string]string
	trialTrafficLimitResetStrategy                            string
	trafficLimitResetStrategy                                 string
	tariffs                                                   []Tariff
	// Trial notifications
	trialInactiveNotificationEnabled bool
	winbackEnabled                   bool
	winbackPrice                     int
	winbackDevices                   int
	winbackMonths                    int
	winbackValidHours                int
	winbackRecurringEnabled          bool
	// Remnawave webhooks
	remnawaveWebhookSecret string
	remnawaveWebhookPath   string
	// Recurring payments
	recurringPaymentsEnabled   bool
	recurringNotifyHoursBefore int
	// Promo tariff codes
	promoTariffCodesEnabled bool
}

var conf config

func RemnawaveTag() string {
	return conf.remnawaveTag
}

func TrialRemnawaveTag() string {
	if conf.trialRemnawaveTag != "" {
		return conf.trialRemnawaveTag
	}
	return conf.remnawaveTag
}

func DefaultLanguage() string {
	return conf.defaultLanguage
}
func GetTributeWebHookUrl() string {
	return conf.tributeWebhookUrl
}
func GetTributeAPIKey() string {
	return conf.tributeAPIKey
}

func GetTributePaymentUrl() string {
	return conf.tributePaymentUrl
}

func GetReferralDays() int {
	return conf.referralDays
}

func GetMiniAppURL() string {
	return conf.miniApp
}

func SquadUUIDs() map[uuid.UUID]uuid.UUID {
	return conf.squadUUIDs
}

func GetBlockedTelegramIds() map[int64]bool {
	return conf.blockedTelegramIds
}

func GetWhitelistedTelegramIds() map[int64]bool {
	return conf.whitelistedTelegramIds
}

func TrialInternalSquads() map[uuid.UUID]uuid.UUID {
	if conf.trialInternalSquads != nil && len(conf.trialInternalSquads) > 0 {
		return conf.trialInternalSquads
	}
	return conf.squadUUIDs
}

func TrialExternalSquadUUID() uuid.UUID {
	if conf.trialExternalSquadUUID != uuid.Nil {
		return conf.trialExternalSquadUUID
	}
	return conf.externalSquadUUID
}

func TrialTrafficLimit() int {
	return conf.trialTrafficLimit * bytesInGigabyte
}

func TrialDays() int {
	return conf.trialDays
}
func FeedbackURL() string {
	return conf.feedbackURL
}

func ChannelURL() string {
	return conf.channelURL
}

func ServerStatusURL() string {
	return conf.serverStatusURL
}

func SupportURL() string {
	return conf.supportURL
}

func TosURL() string {
	return conf.tosURL
}

func YookasaEmail() string {
	return conf.yookasaEmail
}

func Price1() int {
	return conf.price1
}

func Price3() int {
	return conf.price3
}

func Price6() int {
	return conf.price6
}

func Price12() int {
	return conf.price12
}

func DaysInMonth() int {
	return conf.daysInMonth
}

func ExternalSquadUUID() uuid.UUID {
	return conf.externalSquadUUID
}

func Price(month int) int {
	switch month {
	case 1:
		return conf.price1
	case 3:
		return conf.price3
	case 6:
		return conf.price6
	case 12:
		return conf.price12
	default:
		return conf.price1
	}
}

func StarsPrice(month int) int {
	switch month {
	case 1:
		return conf.starsPrice1
	case 3:
		return conf.starsPrice3
	case 6:
		return conf.starsPrice6
	case 12:
		return conf.starsPrice12
	default:
		return conf.starsPrice1
	}
}
func TelegramToken() string {
	return conf.telegramToken
}
func RemnawaveUrl() string {
	return conf.remnawaveUrl
}
func DadaBaseUrl() string {
	return conf.databaseURL
}
func RemnawaveToken() string {
	return conf.remnawaveToken
}
func RemnawaveMode() string {
	return conf.remnawaveMode
}
func CryptoPayUrl() string {
	return conf.cryptoPayURL
}
func CryptoPayToken() string {
	return conf.cryptoPayToken
}
func BotURL() string {
	return conf.botURL
}
func SetBotURL(botURL string) {
	conf.botURL = botURL
}
func YookasaUrl() string {
	return conf.yookasaURL
}
func YookasaShopId() string {
	return conf.yookasaShopId
}
func YookasaSecretKey() string {
	return conf.yookasaSecretKey
}
func TrafficLimit() int {
	return conf.trafficLimit * bytesInGigabyte
}

func IsCryptoPayEnabled() bool {
	return conf.isCryptoEnabled
}

func IsYookasaEnabled() bool {
	return conf.isYookasaEnabled
}

func IsTelegramStarsEnabled() bool {
	return conf.isTelegramStarsEnabled
}

func RequirePaidPurchaseForStars() bool {
	return conf.requirePaidPurchaseForStars
}

func GetAdminTelegramId() int64 {
	return conf.adminTelegramId
}

func GetHealthCheckPort() int {
	return conf.healthCheckPort
}

func IsWepAppLinkEnabled() bool {
	return conf.isWebAppLinkEnabled
}

func IsWebhookEnabled() bool {
	return conf.webhookEnabled
}

func WebhookURL() string {
	return conf.webhookURL
}

func WebhookSecretToken() string {
	return conf.webhookSecretToken
}

func RemnawaveHeaders() map[string]string {
	return conf.remnawaveHeaders
}

func TrialTrafficLimitResetStrategy() string {
	return conf.trialTrafficLimitResetStrategy
}

func TrafficLimitResetStrategy() string {
	return conf.trafficLimitResetStrategy
}

// GetTariffs возвращает все включённые тарифы
func GetTariffs() []Tariff {
	return conf.tariffs
}

// GetTariffByName возвращает тариф по имени или nil если не найден
func GetTariffByName(name string) *Tariff {
	for i := range conf.tariffs {
		if conf.tariffs[i].Name == name {
			return &conf.tariffs[i]
		}
	}
	return nil
}

// GetTariffByTributeName возвращает тариф по названию подписки Tribute или nil если не найден
func GetTariffByTributeName(tributeName string) *Tariff {
	for i := range conf.tariffs {
		if conf.tariffs[i].TributeName != "" && conf.tariffs[i].TributeName == tributeName {
			return &conf.tariffs[i]
		}
	}
	return nil
}

// IsTariffsEnabled возвращает true если есть хотя бы один включённый тариф
func IsTariffsEnabled() bool {
	return len(conf.tariffs) > 0
}

// GetAllTariffDeviceLimits возвращает список всех лимитов устройств из тарифов
// Включает также WINBACK_DEVICES чтобы winback лимит не считался кастомным
func GetAllTariffDeviceLimits() []int {
	// Используем map для уникальности
	limitsMap := make(map[int]bool)
	
	// Добавляем лимиты из тарифов
	for _, t := range conf.tariffs {
		limitsMap[t.Devices] = true
	}
	
	// Добавляем winback devices если включён
	if conf.winbackEnabled && conf.winbackDevices > 0 {
		limitsMap[conf.winbackDevices] = true
	}
	
	// Конвертируем в slice
	limits := make([]int, 0, len(limitsMap))
	for limit := range limitsMap {
		limits = append(limits, limit)
	}
	return limits
}

// Trial notifications functions

// IsTrialInactiveNotificationEnabled возвращает true если уведомления о неактивности триала включены
func IsTrialInactiveNotificationEnabled() bool {
	return conf.trialInactiveNotificationEnabled
}

// IsWinbackEnabled возвращает true если winback предложения включены
func IsWinbackEnabled() bool {
	return conf.winbackEnabled
}

// GetWinbackPrice возвращает цену winback предложения в рублях
func GetWinbackPrice() int {
	return conf.winbackPrice
}

// GetWinbackDevices возвращает лимит устройств для winback предложения
func GetWinbackDevices() int {
	return conf.winbackDevices
}

// GetWinbackMonths возвращает период подписки для winback предложения в месяцах
func GetWinbackMonths() int {
	return conf.winbackMonths
}

// GetWinbackValidHours возвращает срок действия winback предложения в часах
func GetWinbackValidHours() int {
	return conf.winbackValidHours
}

// IsWinbackRecurringEnabled возвращает true если автопродление для winback включено
func IsWinbackRecurringEnabled() bool {
	return conf.winbackRecurringEnabled
}

// GetRemnawaveWebhookSecret возвращает секрет для валидации подписи Remnawave webhooks
func GetRemnawaveWebhookSecret() string {
	return conf.remnawaveWebhookSecret
}

// GetRemnawaveWebhookPath возвращает путь для приёма Remnawave webhooks
func GetRemnawaveWebhookPath() string {
	return conf.remnawaveWebhookPath
}

// IsRecurringPaymentsEnabled возвращает true если рекуррентные платежи включены
func IsRecurringPaymentsEnabled() bool {
	return conf.recurringPaymentsEnabled
}

// GetRecurringNotifyHoursBefore возвращает количество часов до списания для уведомления
func GetRecurringNotifyHoursBefore() int {
	return conf.recurringNotifyHoursBefore
}

// IsPromoTariffCodesEnabled возвращает true если промокоды на тариф включены
func IsPromoTariffCodesEnabled() bool {
	return conf.promoTariffCodesEnabled
}

const bytesInGigabyte = 1073741824

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Panicf("env %q not set", key)
	}
	return v
}

func mustEnvInt(key string) int {
	v := mustEnv(key)
	i, err := strconv.Atoi(v)
	if err != nil {
		log.Panicf("invalid int in %q: %v", key, err)
	}
	return i
}

func envIntDefault(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		log.Panicf("invalid int in %q: %v", key, err)
	}
	return i
}

func envStringDefault(key string, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func envBool(key string) bool {
	return os.Getenv(key) == "true"
}

// parseTariffs парсит тарифы из ENV переменных по паттерну TARIFF_<NAME>_*
// Поддерживает имена с подчёркиванием: TARIFF_SUPER_PRO_ENABLED → name = "SUPER_PRO"
func parseTariffs() []Tariff {
	var tariffs []Tariff
	seen := make(map[string]bool)

	// Известные суффиксы для определения конца имени тарифа
	knownSuffixes := []string{"_ENABLED", "_DEVICES", "_PRICE_1", "_PRICE_3", "_PRICE_6", "_PRICE_12",
		"_STARS_PRICE_1", "_STARS_PRICE_3", "_STARS_PRICE_6", "_STARS_PRICE_12",
		"_TRIBUTE_URL", "_TRIBUTE_NAME"}

	// Собираем все уникальные имена тарифов из ENV
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "TARIFF_") {
			continue
		}
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]

		// Извлекаем имя тарифа: TARIFF_<NAME>_<SUFFIX> → NAME
		// Ищем известный суффикс и отрезаем его
		var name string
		for _, suffix := range knownSuffixes {
			if strings.HasSuffix(key, suffix) {
				// Убираем "TARIFF_" в начале и суффикс в конце
				name = strings.TrimPrefix(key, "TARIFF_")
				name = strings.TrimSuffix(name, suffix)
				break
			}
		}
		if name == "" {
			continue
		}
		seen[name] = true
	}

	// Парсим каждый найденный тариф
	for name := range seen {
		prefix := "TARIFF_" + name + "_"

		// Проверяем включён ли тариф
		if !envBool(prefix + "ENABLED") {
			slog.Debug("Tariff disabled or not enabled", "name", name)
			continue
		}

		// Парсим devices (обязательное поле)
		devicesStr := os.Getenv(prefix + "DEVICES")
		if devicesStr == "" {
			slog.Warn("Tariff missing DEVICES, skipping", "name", name)
			continue
		}
		devices, err := strconv.Atoi(devicesStr)
		if err != nil {
			slog.Warn("Tariff invalid DEVICES value, skipping", "name", name, "error", err)
			continue
		}

		tariff := Tariff{
			Name:    name,
			Devices: devices,
		}

		// Парсим цены (обязательные)
		price1Str := os.Getenv(prefix + "PRICE_1")
		price3Str := os.Getenv(prefix + "PRICE_3")
		price6Str := os.Getenv(prefix + "PRICE_6")
		price12Str := os.Getenv(prefix + "PRICE_12")

		if price1Str == "" || price3Str == "" || price6Str == "" || price12Str == "" {
			slog.Warn("Tariff missing price fields, skipping", "name", name)
			continue
		}

		tariff.Price1, err = strconv.Atoi(price1Str)
		if err != nil {
			slog.Warn("Tariff invalid PRICE_1, skipping", "name", name, "error", err)
			continue
		}
		tariff.Price3, err = strconv.Atoi(price3Str)
		if err != nil {
			slog.Warn("Tariff invalid PRICE_3, skipping", "name", name, "error", err)
			continue
		}
		tariff.Price6, err = strconv.Atoi(price6Str)
		if err != nil {
			slog.Warn("Tariff invalid PRICE_6, skipping", "name", name, "error", err)
			continue
		}
		tariff.Price12, err = strconv.Atoi(price12Str)
		if err != nil {
			slog.Warn("Tariff invalid PRICE_12, skipping", "name", name, "error", err)
			continue
		}

		// Парсим цены в звёздах (опциональные, по умолчанию = обычным ценам)
		tariff.StarsPrice1 = envIntDefault(prefix+"STARS_PRICE_1", tariff.Price1)
		tariff.StarsPrice3 = envIntDefault(prefix+"STARS_PRICE_3", tariff.Price3)
		tariff.StarsPrice6 = envIntDefault(prefix+"STARS_PRICE_6", tariff.Price6)
		tariff.StarsPrice12 = envIntDefault(prefix+"STARS_PRICE_12", tariff.Price12)

		// Парсим Tribute поля (опциональные)
		tariff.TributeURL = os.Getenv(prefix + "TRIBUTE_URL")
		tariff.TributeName = os.Getenv(prefix + "TRIBUTE_NAME")

		tariffs = append(tariffs, tariff)
		slog.Info("Loaded tariff", "name", name, "devices", devices,
			"price1", tariff.Price1, "price3", tariff.Price3,
			"price6", tariff.Price6, "price12", tariff.Price12,
			"tributeURL", tariff.TributeURL != "", "tributeName", tariff.TributeName)
	}

	// Сортируем тарифы по количеству устройств (от меньшего к большему)
	// Это логичный порядок: START (3) → PRO (5) → PREMIUM (10)
	sort.Slice(tariffs, func(i, j int) bool {
		if tariffs[i].Devices != tariffs[j].Devices {
			return tariffs[i].Devices < tariffs[j].Devices
		}
		// При равном количестве устройств — по имени
		return tariffs[i].Name < tariffs[j].Name
	})

	return tariffs
}

func InitConfig() {
	if os.Getenv("DISABLE_ENV_FILE") != "true" {
		if err := godotenv.Load(".env"); err != nil {
			log.Println("No .env loaded:", err)
		}
	}
	var err error
	conf.adminTelegramId, err = strconv.ParseInt(os.Getenv("ADMIN_TELEGRAM_ID"), 10, 64)
	if err != nil {
		panic("ADMIN_TELEGRAM_ID .env variable not set")
	}

	conf.telegramToken = mustEnv("TELEGRAM_TOKEN")

	conf.isWebAppLinkEnabled = func() bool {
		isWebAppLinkEnabled := os.Getenv("IS_WEB_APP_LINK") == "true"
		return isWebAppLinkEnabled
	}()

	conf.miniApp = envStringDefault("MINI_APP_URL", "")

	conf.remnawaveTag = envStringDefault("REMNAWAVE_TAG", "")

	conf.trialRemnawaveTag = envStringDefault("TRIAL_REMNAWAVE_TAG", "")

	conf.trialTrafficLimitResetStrategy = envStringDefault("TRIAL_TRAFFIC_LIMIT_RESET_STRATEGY", "MONTH")
	conf.trafficLimitResetStrategy = envStringDefault("TRAFFIC_LIMIT_RESET_STRATEGY", "MONTH")

	conf.defaultLanguage = envStringDefault("DEFAULT_LANGUAGE", "ru")

	conf.daysInMonth = envIntDefault("DAYS_IN_MONTH", 30)

	externalSquadUUIDStr := os.Getenv("EXTERNAL_SQUAD_UUID")
	if externalSquadUUIDStr != "" {
		parsedUUID, err := uuid.Parse(externalSquadUUIDStr)
		if err != nil {
			panic(fmt.Sprintf("invalid EXTERNAL_SQUAD_UUID format: %v", err))
		}
		conf.externalSquadUUID = parsedUUID
	} else {
		conf.externalSquadUUID = uuid.Nil
	}

	conf.trialTrafficLimit = mustEnvInt("TRIAL_TRAFFIC_LIMIT")

	conf.healthCheckPort = envIntDefault("HEALTH_CHECK_PORT", 8080)

	conf.webhookEnabled = envBool("WEBHOOK_ENABLED")
	if conf.webhookEnabled {
		conf.webhookURL = mustEnv("WEBHOOK_URL")
		conf.webhookSecretToken = envStringDefault("WEBHOOK_SECRET_TOKEN", "")
	}

	conf.trialDays = mustEnvInt("TRIAL_DAYS")

	conf.enableAutoPayment = envBool("ENABLE_AUTO_PAYMENT")

	conf.price1 = mustEnvInt("PRICE_1")
	conf.price3 = mustEnvInt("PRICE_3")
	conf.price6 = mustEnvInt("PRICE_6")
	conf.price12 = mustEnvInt("PRICE_12")

	conf.isTelegramStarsEnabled = envBool("TELEGRAM_STARS_ENABLED")
	if conf.isTelegramStarsEnabled {
		conf.starsPrice1 = envIntDefault("STARS_PRICE_1", conf.price1)
		conf.starsPrice3 = envIntDefault("STARS_PRICE_3", conf.price3)
		conf.starsPrice6 = envIntDefault("STARS_PRICE_6", conf.price6)
		conf.starsPrice12 = envIntDefault("STARS_PRICE_12", conf.price12)

	}

	conf.requirePaidPurchaseForStars = envBool("REQUIRE_PAID_PURCHASE_FOR_STARS")

	conf.remnawaveUrl = mustEnv("REMNAWAVE_URL")

	conf.remnawaveMode = func() string {
		v := os.Getenv("REMNAWAVE_MODE")
		if v != "" {
			if v != "remote" && v != "local" {
				panic("REMNAWAVE_MODE .env variable must be either 'remote' or 'local'")
			} else {
				return v
			}
		} else {
			return "remote"
		}
	}()

	conf.remnawaveToken = mustEnv("REMNAWAVE_TOKEN")

	conf.databaseURL = mustEnv("DATABASE_URL")

	conf.isCryptoEnabled = envBool("CRYPTO_PAY_ENABLED")
	if conf.isCryptoEnabled {
		conf.cryptoPayURL = mustEnv("CRYPTO_PAY_URL")
		conf.cryptoPayToken = mustEnv("CRYPTO_PAY_TOKEN")
	}

	conf.isYookasaEnabled = envBool("YOOKASA_ENABLED")
	if conf.isYookasaEnabled {
		conf.yookasaURL = mustEnv("YOOKASA_URL")
		conf.yookasaShopId = mustEnv("YOOKASA_SHOP_ID")
		conf.yookasaSecretKey = mustEnv("YOOKASA_SECRET_KEY")
		conf.yookasaEmail = mustEnv("YOOKASA_EMAIL")
	}

	conf.trafficLimit = mustEnvInt("TRAFFIC_LIMIT")
	conf.referralDays = mustEnvInt("REFERRAL_DAYS")

	conf.serverStatusURL = os.Getenv("SERVER_STATUS_URL")
	conf.supportURL = os.Getenv("SUPPORT_URL")
	conf.feedbackURL = os.Getenv("FEEDBACK_URL")
	conf.channelURL = os.Getenv("CHANNEL_URL")
	conf.tosURL = os.Getenv("TOS_URL")

	conf.squadUUIDs = func() map[uuid.UUID]uuid.UUID {
		v := os.Getenv("SQUAD_UUIDS")
		if v != "" {
			uuids := strings.Split(v, ",")
			var inboundsMap = make(map[uuid.UUID]uuid.UUID)
			for _, value := range uuids {
				uuid, err := uuid.Parse(value)
				if err != nil {
					panic(err)
				}
				inboundsMap[uuid] = uuid
			}
			slog.Info("Loaded squad UUIDs", "uuids", uuids)
			return inboundsMap
		} else {
			slog.Info("No squad UUIDs specified, all will be used")
			return map[uuid.UUID]uuid.UUID{}
		}
	}()

	conf.tributeWebhookUrl = os.Getenv("TRIBUTE_WEBHOOK_URL")
	if conf.tributeWebhookUrl != "" {
		conf.tributeAPIKey = mustEnv("TRIBUTE_API_KEY")
		conf.tributePaymentUrl = mustEnv("TRIBUTE_PAYMENT_URL")
	}

	conf.blockedTelegramIds = func() map[int64]bool {
		v := os.Getenv("BLOCKED_TELEGRAM_IDS")
		if v != "" {
			ids := strings.Split(v, ",")
			var blockedMap = make(map[int64]bool)
			for _, idStr := range ids {
				id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
				if err != nil {
					panic(fmt.Sprintf("invalid telegram ID in BLOCKED_TELEGRAM_IDS: %v", err))
				}
				blockedMap[id] = true
			}
			slog.Info("Loaded blocked telegram IDs", "count", len(blockedMap))
			return blockedMap
		} else {
			slog.Info("No blocked telegram IDs specified")
			return map[int64]bool{}
		}
	}()

	conf.whitelistedTelegramIds = func() map[int64]bool {
		v := os.Getenv("WHITELISTED_TELEGRAM_IDS")
		if v != "" {
			ids := strings.Split(v, ",")
			var whitelistedMap = make(map[int64]bool)
			for _, idStr := range ids {
				id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
				if err != nil {
					panic(fmt.Sprintf("invalid telegram ID in WHITELISTED_TELEGRAM_IDS: %v", err))
				}
				whitelistedMap[id] = true
			}
			slog.Info("Loaded whitelisted telegram IDs", "count", len(whitelistedMap))
			return whitelistedMap
		} else {
			slog.Info("No whitelisted telegram IDs specified")
			return map[int64]bool{}
		}
	}()

	conf.trialInternalSquads = func() map[uuid.UUID]uuid.UUID {
		v := os.Getenv("TRIAL_INTERNAL_SQUADS")
		if v != "" {
			uuids := strings.Split(v, ",")
			var trialSquadsMap = make(map[uuid.UUID]uuid.UUID)
			for _, value := range uuids {
				parsedUUID, err := uuid.Parse(strings.TrimSpace(value))
				if err != nil {
					panic(fmt.Sprintf("invalid UUID in TRIAL_INTERNAL_SQUADS: %v", err))
				}
				trialSquadsMap[parsedUUID] = parsedUUID
			}
			slog.Info("Loaded trial internal squad UUIDs", "uuids", uuids)
			return trialSquadsMap
		} else {
			slog.Info("No trial internal squads specified, will use regular SQUAD_UUIDS for trial users")
			return map[uuid.UUID]uuid.UUID{}
		}
	}()

	trialExternalSquadUUIDStr := os.Getenv("TRIAL_EXTERNAL_SQUAD_UUID")
	if trialExternalSquadUUIDStr != "" {
		parsedUUID, err := uuid.Parse(trialExternalSquadUUIDStr)
		if err != nil {
			panic(fmt.Sprintf("invalid TRIAL_EXTERNAL_SQUAD_UUID format: %v", err))
		}
		conf.trialExternalSquadUUID = parsedUUID
		slog.Info("Loaded trial external squad UUID", "uuid", trialExternalSquadUUIDStr)
	} else {
		conf.trialExternalSquadUUID = uuid.Nil
		slog.Info("No trial external squad specified, will use regular EXTERNAL_SQUAD_UUID for trial users")
	}

	conf.remnawaveHeaders = func() map[string]string {
		v := os.Getenv("REMNAWAVE_HEADERS")
		if v != "" {
			headers := make(map[string]string)
			pairs := strings.Split(v, ";")
			for _, pair := range pairs {
				parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					if key != "" && value != "" {
						headers[key] = value
					}
				}
			}
			if len(headers) > 0 {
				slog.Info("Loaded remnawave headers", "count", len(headers))
				return headers
			}
		}
		return map[string]string{}
	}()

	// Парсим тарифы из ENV
	conf.tariffs = parseTariffs()
	if len(conf.tariffs) > 0 {
		slog.Info("Tariffs system enabled", "count", len(conf.tariffs))
	} else {
		slog.Info("No tariffs configured, using legacy pricing")
	}

	// Trial notifications config
	conf.trialInactiveNotificationEnabled = envBool("TRIAL_INACTIVE_NOTIFICATION_ENABLED")
	conf.winbackEnabled = envBool("WINBACK_ENABLED")
	conf.winbackPrice = envIntDefault("WINBACK_PRICE", 100)
	conf.winbackDevices = envIntDefault("WINBACK_DEVICES", 1)
	conf.winbackMonths = envIntDefault("WINBACK_MONTHS", 1)
	conf.winbackValidHours = envIntDefault("WINBACK_VALID_HOURS", 48)
	conf.winbackRecurringEnabled = envBool("WINBACK_RECURRING_ENABLED")

	if conf.trialInactiveNotificationEnabled {
		slog.Info("Trial inactive notification enabled")
	}
	if conf.winbackEnabled {
		slog.Info("Winback offers enabled",
			"price", conf.winbackPrice,
			"devices", conf.winbackDevices,
			"months", conf.winbackMonths,
			"validHours", conf.winbackValidHours)
	}

	// Remnawave webhooks config
	conf.remnawaveWebhookSecret = os.Getenv("REMNAWAVE_WEBHOOK_SECRET")
	conf.remnawaveWebhookPath = envStringDefault("REMNAWAVE_WEBHOOK_PATH", "/remnawave-webhook")
	if conf.remnawaveWebhookSecret != "" {
		slog.Info("Remnawave webhooks enabled", "path", conf.remnawaveWebhookPath)
	}

	// Recurring payments config
	conf.recurringPaymentsEnabled = envBool("RECURRING_PAYMENTS_ENABLED")
	conf.recurringNotifyHoursBefore = envIntDefault("RECURRING_NOTIFY_HOURS_BEFORE", 48)
	if conf.recurringPaymentsEnabled {
		slog.Info("Recurring payments enabled", "notifyHoursBefore", conf.recurringNotifyHoursBefore)
	}

	// Promo tariff codes config
	conf.promoTariffCodesEnabled = envBool("PROMO_TARIFF_CODES_ENABLED")
	if conf.promoTariffCodesEnabled {
		slog.Info("Promo tariff codes enabled")
	}
}
