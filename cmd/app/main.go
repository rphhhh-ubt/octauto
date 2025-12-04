package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"remnawave-tg-shop-bot/internal/broadcast"
	"remnawave-tg-shop-bot/internal/cache"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/cryptopay"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/handler"
	"remnawave-tg-shop-bot/internal/notification"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/promo"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/sync"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/internal/tribute"
	"remnawave-tg-shop-bot/internal/yookasa"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/robfig/cron/v3"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	config.InitConfig()
	slog.Info("Application starting", "version", Version, "commit", Commit, "buildDate", BuildDate)

	tm := translation.GetInstance()
	err := tm.InitTranslations("./translations", config.DefaultLanguage())
	if err != nil {
		panic(err)
	}

	pool, err := initDatabase(ctx, config.DadaBaseUrl())
	if err != nil {
		panic(err)
	}

	err = database.RunMigrations(ctx, &database.MigrationConfig{Direction: "up", MigrationsPath: "./db/migrations", Steps: 0}, pool)
	if err != nil {
		panic(err)
	}
	cache := cache.NewCache(30 * time.Minute)
	customerRepository := database.NewCustomerRepository(pool)
	purchaseRepository := database.NewPurchaseRepository(pool)
	referralRepository := database.NewReferralRepository(pool)
	promoRepository := database.NewPromoRepository(pool)

	cryptoPayClient := cryptopay.NewCryptoPayClient(config.CryptoPayUrl(), config.CryptoPayToken())
	remnawaveClient := remnawave.NewClient(config.RemnawaveUrl(), config.RemnawaveToken(), config.RemnawaveMode())
	yookasaClient := yookasa.NewClient(config.YookasaUrl(), config.YookasaShopId(), config.YookasaSecretKey())
	botOpts := []bot.Option{bot.WithWorkers(3)}
	if config.IsWebhookEnabled() && config.WebhookSecretToken() != "" {
		botOpts = append(botOpts, bot.WithWebhookSecretToken(config.WebhookSecretToken()))
	}
	b, err := bot.New(config.TelegramToken(), botOpts...)
	if err != nil {
		panic(err)
	}

	paymentService := payment.NewPaymentService(tm, purchaseRepository, remnawaveClient, customerRepository, b, cryptoPayClient, yookasaClient, referralRepository, cache)

	cronScheduler := setupInvoiceChecker(purchaseRepository, cryptoPayClient, paymentService, yookasaClient, customerRepository)
	if cronScheduler != nil {
		cronScheduler.Start()
		defer cronScheduler.Stop()
	}

	subService := notification.NewSubscriptionService(customerRepository, purchaseRepository, paymentService, b, tm)
	remnawaveAdapter := notification.NewRemnawaveClientAdapter(remnawaveClient)
	subService.SetRemnawaveClient(remnawaveAdapter)

	// Устанавливаем сервис для тестирования уведомлений из админки
	handler.SetNotificationTester(subService)

	subscriptionNotificationCronScheduler := subscriptionChecker(subService)
	subscriptionNotificationCronScheduler.Start()
	defer subscriptionNotificationCronScheduler.Stop()

	syncService := sync.NewSyncService(remnawaveClient, customerRepository)

	broadcastRepo := database.NewBroadcastRepository(pool)
	broadcastService := broadcast.NewBroadcastService(b, customerRepository, broadcastRepo)

	promoService := promo.NewService(promoRepository, customerRepository, remnawaveClient)

	// Promo tariff service
	promoTariffRepo := database.NewPromoTariffRepository(pool)
	promoTariffService := promo.NewTariffService(promoTariffRepo, customerRepository)

	h := handler.NewHandler(syncService, paymentService, tm, customerRepository, purchaseRepository, cryptoPayClient, yookasaClient, referralRepository, cache, broadcastService, promoService, promoTariffService, remnawaveClient)

	me, err := b.GetMe(ctx)
	if err != nil {
		panic(err)
	}

	_, err = b.SetChatMenuButton(ctx, &bot.SetChatMenuButtonParams{
		MenuButton: &models.MenuButtonCommands{
			Type: models.MenuButtonTypeCommands,
		},
	})

	if err != nil {
		panic(err)
	}
	_, err = b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			{Command: "start", Description: "Начать работу с ботом"},
		},
		LanguageCode: "ru",
	})

	_, err = b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			{Command: "start", Description: "Start using the bot"},
		},
		LanguageCode: "en",
	})

	config.SetBotURL(fmt.Sprintf("https://t.me/%s", me.Username))

	b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypePrefix, h.StartCommandHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/connect", bot.MatchTypeExact, h.ConnectCommandHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/sync", bot.MatchTypeExact, h.SyncUsersCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/admin", bot.MatchTypeExact, h.AdminCommandHandler, isAdminMiddleware)

	// Promo code handlers
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromo, bot.MatchTypeExact, h.PromoCodeCallbackHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "bc_promo", bot.MatchTypeExact, h.BroadcastPromoCallbackHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "bc_buy", bot.MatchTypeExact, h.BroadcastBuyCallbackHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_promo", bot.MatchTypeExact, h.AdminPromoCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_promo_create", bot.MatchTypeExact, h.AdminPromoCreateCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_promo_list", bot.MatchTypeExact, h.AdminPromoListCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_promo_view_", bot.MatchTypePrefix, h.AdminPromoViewCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_promo_delete_", bot.MatchTypePrefix, h.AdminPromoDeleteCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_promo_activate_", bot.MatchTypePrefix, h.AdminPromoToggleCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_promo_deactivate_", bot.MatchTypePrefix, h.AdminPromoToggleCallback, isAdminMiddleware)

	// Promo tariff handlers (admin)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_promo_tariff", bot.MatchTypeExact, h.AdminPromoTariffCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_promo_tariff_create", bot.MatchTypeExact, h.AdminPromoTariffCreateCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_promo_tariff_list", bot.MatchTypeExact, h.AdminPromoTariffListCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_promo_tariff_view_", bot.MatchTypePrefix, h.AdminPromoTariffViewCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_promo_tariff_delete_", bot.MatchTypePrefix, h.AdminPromoTariffDeleteCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_promo_tariff_activate_", bot.MatchTypePrefix, h.AdminPromoTariffToggleCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_promo_tariff_deactivate_", bot.MatchTypePrefix, h.AdminPromoTariffToggleCallback, isAdminMiddleware)

	// Promo tariff user handler - Requirements: 5.3
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPromoTariff, bot.MatchTypeExact, h.PromoTariffCallbackHandler, h.SuspiciousUserFilterMiddleware)

	// Broadcast handlers
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_broadcast", bot.MatchTypeExact, h.AdminBroadcastCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "broadcast_target_", bot.MatchTypePrefix, h.AdminBroadcastTargetCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "broadcast_btn_", bot.MatchTypePrefix, h.AdminBroadcastButtonCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "broadcast_confirm_", bot.MatchTypePrefix, h.AdminBroadcastConfirmCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_broadcast_history", bot.MatchTypeExact, h.AdminBroadcastHistoryCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "broadcast_view_", bot.MatchTypePrefix, h.AdminBroadcastViewCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "broadcast_delete_", bot.MatchTypePrefix, h.AdminBroadcastDeleteCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_back", bot.MatchTypeExact, h.AdminBackCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_close", bot.MatchTypeExact, h.AdminCloseCallback, isAdminMiddleware)

	// Test notifications handlers
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_test_notifications", bot.MatchTypeExact, h.AdminTestNotificationsCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_test_inactive_trial", bot.MatchTypeExact, h.AdminTestInactiveTrialCallback, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin_test_winback", bot.MatchTypeExact, h.AdminTestWinbackCallback, isAdminMiddleware)
	
	// Обработчик текста и медиа для рассылки и создания промокодов (только для админа)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		if update.Message == nil || update.Message.From.ID != config.GetAdminTelegramId() {
			return false
		}
		// Текст (не команда), фото, GIF, видео или кружок
		hasText := update.Message.Text != "" && !strings.HasPrefix(update.Message.Text, "/")
		hasPhoto := update.Message.Photo != nil && len(update.Message.Photo) > 0
		hasAnimation := update.Message.Animation != nil
		hasVideo := update.Message.Video != nil
		hasVideoNote := update.Message.VideoNote != nil
		return hasText || hasPhoto || hasAnimation || hasVideo || hasVideoNote
	}, h.AdminTextInputHandler)

	// Обработчик ввода промокода от пользователя (только если есть состояние ожидания)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		if update.Message == nil {
			return false
		}
		if update.Message.Text == "" || strings.HasPrefix(update.Message.Text, "/") {
			return false
		}
		// Проверяем состояние - только если пользователь в режиме ввода промокода
		stateKey := fmt.Sprintf("promo_state_%d", update.Message.From.ID)
		state, found := cache.GetString(stateKey)
		return found && state == "waiting_code"
	}, h.PromoCodeInputHandler, h.SuspiciousUserFilterMiddleware)

	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackReferral, bot.MatchTypeExact, h.ReferralCallbackHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBuy, bot.MatchTypeExact, h.BuyCallbackHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackTariff, bot.MatchTypePrefix, h.TariffCallbackHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackTrial, bot.MatchTypeExact, h.TrialCallbackHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackActivateTrial, bot.MatchTypeExact, h.ActivateTrialCallbackHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackWinbackActivate, bot.MatchTypeExact, h.WinbackCallbackHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackStart, bot.MatchTypeExact, h.StartCallbackHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackSell, bot.MatchTypePrefix, h.SellCallbackHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackConnect, bot.MatchTypeExact, h.ConnectCallbackHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPayment, bot.MatchTypePrefix, h.PaymentCallbackHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackRecurringToggle, bot.MatchTypePrefix, h.RecurringToggleCallbackHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackRecurringDisable, bot.MatchTypeExact, h.RecurringDisableCallbackHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackDeletePaymentMethod, bot.MatchTypeExact, h.DeletePaymentMethodCallbackHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackSavedPaymentMethods, bot.MatchTypeExact, h.SavedPaymentMethodsCallbackHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackCloseMessage, bot.MatchTypeExact, h.CloseMessageCallbackHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.PreCheckoutQuery != nil
	}, h.PreCheckoutCallbackHandler, h.SuspiciousUserFilterMiddleware)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil && update.Message.SuccessfulPayment != nil
	}, h.SuccessPaymentHandler, h.SuspiciousUserFilterMiddleware)

	mux := http.NewServeMux()
	mux.Handle("/healthcheck", fullHealthHandler(pool, remnawaveClient))
	if config.GetTributeWebHookUrl() != "" {
		tributeHandler := tribute.NewClient(paymentService, customerRepository)
		mux.Handle(config.GetTributeWebHookUrl(), tributeHandler.WebHookHandler())
	}

	// Remnawave webhook handler для уведомлений об истечении подписки, winback и автопродления
	// Requirements: 3.2, 2.1, 2.2, 2.3, 2.4, 2.5
	if config.GetRemnawaveWebhookSecret() != "" {
		remnawaveWebhookHandler := handler.NewRemnawaveWebhookHandler(tm, b, customerRepository, purchaseRepository)
		// Устанавливаем клиенты для рекуррентных платежей
		if config.IsRecurringPaymentsEnabled() && config.IsYookasaEnabled() {
			remnawaveWebhookHandler.SetYookasaClient(yookasaClient)
			remnawaveWebhookHandler.SetRemnawaveClient(remnawaveClient)
			slog.Info("Recurring payments enabled for webhook handler")
		}
		mux.HandleFunc(config.GetRemnawaveWebhookPath(), remnawaveWebhookHandler.HandleWebhook)
		slog.Info("Remnawave webhook handler registered", "path", config.GetRemnawaveWebhookPath())
	}

	// Webhook mode
	if config.IsWebhookEnabled() {
		mux.Handle("/webhook", b.WebhookHandler())
		
		srv := &http.Server{
			Addr:    fmt.Sprintf(":%d", config.GetHealthCheckPort()),
			Handler: mux,
		}

		// Set webhook
		_, err = b.SetWebhook(ctx, &bot.SetWebhookParams{
			URL:            config.WebhookURL(),
			SecretToken:    config.WebhookSecretToken(),
			AllowedUpdates: []string{"message", "callback_query", "pre_checkout_query"},
		})
		if err != nil {
			panic(fmt.Sprintf("Failed to set webhook: %v", err))
		}
		slog.Info("Webhook set", "url", config.WebhookURL())

		go b.StartWebhook(ctx)

		go func() {
			log.Printf("Server listening on %s (webhook mode)", srv.Addr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Server error: %v", err)
			}
		}()

		<-ctx.Done()

		// Delete webhook on shutdown
		_, _ = b.DeleteWebhook(context.Background(), &bot.DeleteWebhookParams{})
		slog.Info("Webhook deleted")

		log.Println("Shutting down server…")
		shutdownCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	} else {
		// Polling mode (original)
		srv := &http.Server{
			Addr:    fmt.Sprintf(":%d", config.GetHealthCheckPort()),
			Handler: mux,
		}
		go func() {
			log.Printf("Server listening on %s (polling mode)", srv.Addr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Server error: %v", err)
			}
		}()

		slog.Info("Bot is starting...")
		b.Start(ctx)

		log.Println("Shutting down health server…")
		shutdownCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Health server shutdown error: %v", err)
		}
	}

}

func fullHealthHandler(pool *pgxpool.Pool, rw *remnawave.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := map[string]string{
			"status":    "ok",
			"db":        "ok",
			"rw":        "ok",
			"time":      time.Now().Format(time.RFC3339),
			"version":   Version,
			"commit":    Commit,
			"buildDate": BuildDate,
		}

		dbCtx, dbCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer dbCancel()
		if err := pool.Ping(dbCtx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			status["status"] = "fail"
			status["db"] = "error: " + err.Error()
		}

		rwCtx, rwCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer rwCancel()
		if err := rw.Ping(rwCtx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			status["status"] = "fail"
			status["rw"] = "error: " + err.Error()
		}

		if status["status"] == "ok" {
			w.WriteHeader(http.StatusOK)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"%s","db":"%s","remnawave":"%s","time":"%s","version":"%s","commit":"%s","buildDate":"%s"}`,
			status["status"], status["db"], status["rw"], status["time"], Version, Commit, BuildDate)
	})
}

func isAdminMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		adminID := config.GetAdminTelegramId()
		
		if update.Message != nil && update.Message.From.ID == adminID {
			next(ctx, b, update)
			return
		}
		
		if update.CallbackQuery != nil && update.CallbackQuery.From.ID == adminID {
			next(ctx, b, update)
			return
		}
	}
}

func subscriptionChecker(subService *notification.SubscriptionService) *cron.Cron {
	c := cron.New()

	// Проверка неактивных триальных пользователей каждый час
	// Requirements: 2.1, 3.1
	_, err := c.AddFunc("0 * * * *", func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Panic in ProcessTrialInactiveNotifications", "panic", r)
			}
		}()
		err := subService.ProcessTrialInactiveNotifications()
		if err != nil {
			slog.Error("Error processing trial inactive notifications", "error", err)
		}
	})
	if err != nil {
		panic(err)
	}

	// Winback теперь обрабатывается через вебхук user.expired_24_hours_ago от Remnawave

	return c
}

func initDatabase(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}

	config.MaxConns = 20
	config.MinConns = 5

	return pgxpool.ConnectConfig(ctx, config)
}

func setupInvoiceChecker(
	purchaseRepository *database.PurchaseRepository,
	cryptoPayClient *cryptopay.Client,
	paymentService *payment.PaymentService,
	yookasaClient *yookasa.Client,
	customerRepository *database.CustomerRepository) *cron.Cron {
	if !config.IsYookasaEnabled() && !config.IsCryptoPayEnabled() {
		return nil
	}
	c := cron.New(cron.WithSeconds())

	if config.IsCryptoPayEnabled() {
		_, err := c.AddFunc("*/5 * * * * *", func() {
			ctx := context.Background()
			checkCryptoPayInvoice(ctx, purchaseRepository, cryptoPayClient, paymentService)
		})

		if err != nil {
			panic(err)
		}
	}

	if config.IsYookasaEnabled() {
		// Проверяем каждые 10 секунд (было 5) чтобы не перегружать API
		_, err := c.AddFunc("*/10 * * * * *", func() {
			ctx := context.Background()
			checkYookasaInvoice(ctx, purchaseRepository, yookasaClient, paymentService, customerRepository)
		})

		if err != nil {
			panic(err)
		}
	}

	return c
}

func checkYookasaInvoice(
	ctx context.Context,
	purchaseRepository *database.PurchaseRepository,
	yookasaClient *yookasa.Client,
	paymentService *payment.PaymentService,
	customerRepository *database.CustomerRepository,
) {
	pendingPurchases, err := purchaseRepository.FindByInvoiceTypeAndStatus(
		ctx,
		database.InvoiceTypeYookasa,
		database.PurchaseStatusPending,
	)
	if err != nil {
		log.Printf("Error finding pending purchases: %v", err)
		return
	}
	if len(*pendingPurchases) == 0 {
		return
	}

	for i, purchase := range *pendingPurchases {
		// Задержка между запросами чтобы не перегружать API ЮКассы
		if i > 0 {
			time.Sleep(200 * time.Millisecond)
		}

		invoice, err := yookasaClient.GetPayment(ctx, *purchase.YookasaID)

		if err != nil {
			slog.Error("Error getting invoice", "invoiceId", purchase.YookasaID, "error", err)
			continue
		}

		if invoice.IsCancelled() {
			err := paymentService.CancelYookassaPayment(purchase.ID)
			if err != nil {
				slog.Error("Error canceling invoice", "invoiceId", invoice.ID, "purchaseId", purchase.ID, "error", err)
			}
			continue
		}

		if !invoice.Paid {
			continue
		}

		purchaseId, err := strconv.Atoi(invoice.Metadata["purchaseId"])
		if err != nil {
			slog.Error("Error parsing purchaseId", "invoiceId", invoice.ID, "error", err)
		}
		ctxWithValue := context.WithValue(ctx, "username", invoice.Metadata["username"])
		err = paymentService.ProcessPurchaseById(ctxWithValue, int64(purchaseId))
		if err != nil {
			slog.Error("Error processing invoice", "invoiceId", invoice.ID, "purchaseId", purchaseId, "error", err)
		} else {
			slog.Info("Invoice processed", "invoiceId", invoice.ID, "purchaseId", purchaseId)
		}

		// Сохраняем payment_method_id если способ оплаты был сохранён для рекуррентных платежей
		// Requirements: 1.3
		if invoice.IsPaymentMethodSaved() {
			saveRecurringPaymentMethod(ctx, invoice, purchase.CustomerID, customerRepository)
		}

	}
}

// saveRecurringPaymentMethod сохраняет payment_method_id и настройки рекуррентных платежей
// Requirements: 1.3
func saveRecurringPaymentMethod(
	ctx context.Context,
	invoice *yookasa.Payment,
	customerID int64,
	customerRepository *database.CustomerRepository,
) {
	paymentMethodID := invoice.GetPaymentMethodID().String()

	// Получаем настройки recurring из метаданных платежа
	var tariffName *string
	var months *int
	var amount *int

	if tn, ok := invoice.Metadata["recurring_tariff_name"]; ok && tn != "" {
		tariffName = &tn
	}

	if m, ok := invoice.Metadata["recurring_months"]; ok {
		if monthsInt, err := strconv.Atoi(m); err == nil {
			months = &monthsInt
		}
	}

	if a, ok := invoice.Metadata["recurring_amount"]; ok {
		if amountInt, err := strconv.Atoi(a); err == nil {
			amount = &amountInt
		}
	}

	err := customerRepository.UpdateRecurringSettings(
		ctx,
		customerID,
		true, // recurring_enabled
		&paymentMethodID,
		tariffName,
		months,
		amount,
	)

	if err != nil {
		slog.Error("Error saving recurring payment method",
			"customerID", customerID,
			"paymentMethodID", paymentMethodID,
			"error", err)
	} else {
		slog.Info("Recurring payment method saved",
			"customerID", customerID,
			"paymentMethodID", paymentMethodID,
			"tariffName", tariffName,
			"months", months,
			"amount", amount)
	}
}

func checkCryptoPayInvoice(
	ctx context.Context,
	purchaseRepository *database.PurchaseRepository,
	cryptoPayClient *cryptopay.Client,
	paymentService *payment.PaymentService,
) {
	pendingPurchases, err := purchaseRepository.FindByInvoiceTypeAndStatus(
		ctx,
		database.InvoiceTypeCrypto,
		database.PurchaseStatusPending,
	)
	if err != nil {
		log.Printf("Error finding pending purchases: %v", err)
		return
	}
	if len(*pendingPurchases) == 0 {
		return
	}

	var invoiceIDs []string

	for _, purchase := range *pendingPurchases {
		if purchase.CryptoInvoiceID != nil {
			invoiceIDs = append(invoiceIDs, fmt.Sprintf("%d", *purchase.CryptoInvoiceID))
		}
	}

	if len(invoiceIDs) == 0 {
		return
	}

	stringInvoiceIDs := strings.Join(invoiceIDs, ",")
	invoices, err := cryptoPayClient.GetInvoices("", "", "", stringInvoiceIDs, 0, 0)
	if err != nil {
		log.Printf("Error getting invoices: %v", err)
		return
	}

	for _, invoice := range *invoices {
		if invoice.InvoiceID != nil && invoice.IsPaid() {
			payload := strings.Split(invoice.Payload, "&")
			purchaseID, err := strconv.Atoi(strings.Split(payload[0], "=")[1])
			username := strings.Split(payload[1], "=")[1]
			ctxWithUsername := context.WithValue(ctx, "username", username)
			err = paymentService.ProcessPurchaseById(ctxWithUsername, int64(purchaseID))
			if err != nil {
				slog.Error("Error processing invoice", "invoiceId", invoice.InvoiceID, "error", err)
			} else {
				slog.Info("Invoice processed", "invoiceId", invoice.InvoiceID, "purchaseId", purchaseID)
			}

		}
	}
}
