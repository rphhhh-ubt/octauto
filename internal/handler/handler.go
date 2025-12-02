package handler

import (
	"context"
	"time"

	"remnawave-tg-shop-bot/internal/broadcast"
	"remnawave-tg-shop-bot/internal/cache"
	"remnawave-tg-shop-bot/internal/cryptopay"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/promo"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/sync"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/internal/yookasa"
)

// BroadcastService interface для избежания циклических импортов
type BroadcastService interface {
	CreateBroadcast(ctx context.Context, targetType, messageText string) (int64, error)
	StartBroadcast(ctx context.Context, broadcastID int64, targetType, messageText string)
	StartBroadcastWithOptions(ctx context.Context, broadcastID int64, targetType, messageText string, opts *broadcast.BroadcastOptions)
	GetTargetCustomersCount(ctx context.Context, targetType string) (int, error)
	GetBroadcast(ctx context.Context, id int64) (*database.BroadcastHistory, error)
	GetBroadcastHistory(ctx context.Context, limit, offset int) ([]database.BroadcastHistory, error)
	DeleteBroadcast(ctx context.Context, id int64) error
}

// PromoServiceInterface interface для промокодов
type PromoServiceInterface interface {
	ApplyPromoCode(ctx context.Context, customerID int64, telegramID int64, code string) *promo.ApplyResult
	CreatePromoCode(ctx context.Context, code string, bonusDays, maxActivations int, adminID int64, validUntil *time.Time) (*database.PromoCode, error)
	GetAllPromoCodes(ctx context.Context, limit, offset int) ([]database.PromoCode, error)
	GetPromoByID(ctx context.Context, id int64) (*database.PromoCode, error)
	DeactivatePromo(ctx context.Context, promoID int64) error
	ActivatePromo(ctx context.Context, promoID int64) error
	DeletePromo(ctx context.Context, promoID int64) error
}

// PromoTariffServiceInterface interface для промокодов на тариф
type PromoTariffServiceInterface interface {
	ApplyPromoTariffCode(ctx context.Context, customerID int64, code string) *promo.TariffApplyResult
	CreatePromoTariffCode(ctx context.Context, code string, price, devices, months, maxActivations, validHours int, adminID int64, validUntil *time.Time) (*database.PromoTariffCode, error)
	GetAllPromoTariffCodes(ctx context.Context, limit, offset int) ([]database.PromoTariffCode, error)
	GetPromoTariffByID(ctx context.Context, id int64) (*database.PromoTariffCode, error)
	DeactivatePromoTariff(ctx context.Context, promoID int64) error
	ActivatePromoTariff(ctx context.Context, promoID int64) error
	DeletePromoTariff(ctx context.Context, promoID int64) error
}

type Handler struct {
	customerRepository  *database.CustomerRepository
	purchaseRepository  *database.PurchaseRepository
	cryptoPayClient     *cryptopay.Client
	yookasaClient       *yookasa.Client
	translation         *translation.Manager
	paymentService      *payment.PaymentService
	syncService         *sync.SyncService
	referralRepository  *database.ReferralRepository
	cache               *cache.Cache
	broadcastService    BroadcastService
	promoService        PromoServiceInterface
	promoTariffService  PromoTariffServiceInterface
	remnawaveClient     *remnawave.Client
}

func NewHandler(
	syncService *sync.SyncService,
	paymentService *payment.PaymentService,
	translation *translation.Manager,
	customerRepository *database.CustomerRepository,
	purchaseRepository *database.PurchaseRepository,
	cryptoPayClient *cryptopay.Client,
	yookasaClient *yookasa.Client,
	referralRepository *database.ReferralRepository,
	cache *cache.Cache,
	broadcastService BroadcastService,
	promoService PromoServiceInterface,
	promoTariffService PromoTariffServiceInterface,
	remnawaveClient *remnawave.Client,
) *Handler {
	return &Handler{
		syncService:        syncService,
		paymentService:     paymentService,
		customerRepository: customerRepository,
		purchaseRepository: purchaseRepository,
		cryptoPayClient:    cryptoPayClient,
		yookasaClient:      yookasaClient,
		translation:        translation,
		referralRepository: referralRepository,
		cache:              cache,
		broadcastService:   broadcastService,
		promoService:       promoService,
		promoTariffService: promoTariffService,
		remnawaveClient:    remnawaveClient,
	}
}
