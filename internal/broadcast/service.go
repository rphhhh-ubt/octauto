package broadcast

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/utils"
)

// MediaType типы медиа для broadcast
const (
	MediaTypePhoto     = "photo"
	MediaTypeGIF       = "gif"
	MediaTypeVideo     = "video"
	MediaTypeVideoNote = "video_note"
)

// BroadcastOptions содержит опции для рассылки
type BroadcastOptions struct {
	MediaType   string   // тип медиа: "photo", "gif", "video", "video_note"
	MediaFileID string   // file_id медиа (опционально)
	Buttons     []string // список кнопок: "promo", "subscription", "buy"
	MiniAppURL  string   // URL mini app для кнопки "Ваша подписка"
}

type BroadcastService struct {
	bot                *bot.Bot
	customerRepository *database.CustomerRepository
	broadcastRepo      *database.BroadcastRepository
	mu                 sync.Mutex
	runningBroadcasts  map[int64]bool
}

func NewBroadcastService(
	b *bot.Bot,
	customerRepository *database.CustomerRepository,
	broadcastRepo *database.BroadcastRepository,
) *BroadcastService {
	return &BroadcastService{
		bot:                b,
		customerRepository: customerRepository,
		broadcastRepo:      broadcastRepo,
		runningBroadcasts:  make(map[int64]bool),
	}
}

func (s *BroadcastService) CreateBroadcast(ctx context.Context, targetType, messageText string) (int64, error) {
	return s.broadcastRepo.Create(ctx, targetType, messageText)
}

// GetTargetCustomersCount возвращает количество получателей для указанного типа рассылки
func (s *BroadcastService) GetTargetCustomersCount(ctx context.Context, targetType string) (int, error) {
	customers, err := s.getTargetCustomers(ctx, targetType)
	if err != nil {
		return 0, err
	}
	return len(customers), nil
}

func (s *BroadcastService) StartBroadcast(ctx context.Context, broadcastID int64, targetType, messageText string) {
	s.StartBroadcastWithOptions(ctx, broadcastID, targetType, messageText, nil)
}

func (s *BroadcastService) StartBroadcastWithOptions(ctx context.Context, broadcastID int64, targetType, messageText string, opts *BroadcastOptions) {
	s.mu.Lock()
	if s.runningBroadcasts[broadcastID] {
		s.mu.Unlock()
		slog.Warn("Broadcast already running", "id", broadcastID)
		return
	}
	s.runningBroadcasts[broadcastID] = true
	s.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Panic in broadcast", r, "id", broadcastID)
				bgCtx := context.Background()
				_ = s.broadcastRepo.UpdateStatus(bgCtx, broadcastID, string(database.BroadcastStatusFailed), 0, 0)
			}
			s.mu.Lock()
			delete(s.runningBroadcasts, broadcastID)
			s.mu.Unlock()
		}()

		// Создаем новый контекст для background задачи
		bgCtx := context.Background()
		err := s.executeBroadcastWithOptions(bgCtx, broadcastID, targetType, messageText, opts)
		if err != nil {
			slog.Error("Broadcast execution failed", "error", err, "id", broadcastID)
		}
	}()
}

func (s *BroadcastService) executeBroadcastWithOptions(ctx context.Context, broadcastID int64, targetType, messageText string, opts *BroadcastOptions) error {
	customers, err := s.getTargetCustomers(ctx, targetType)
	if err != nil {
		_ = s.broadcastRepo.UpdateStatus(ctx, broadcastID, string(database.BroadcastStatusFailed), 0, 0)
		return fmt.Errorf("failed to get customers: %w", err)
	}

	totalCount := len(customers)
	err = s.broadcastRepo.SetTotalCount(ctx, broadcastID, totalCount)
	if err != nil {
		return fmt.Errorf("failed to set total count: %w", err)
	}

	if totalCount == 0 {
		_ = s.broadcastRepo.UpdateStatus(ctx, broadcastID, string(database.BroadcastStatusCompleted), 0, 0)
		return nil
	}

	// Подготавливаем клавиатуру если есть кнопки
	var keyboard *models.InlineKeyboardMarkup
	if opts != nil && len(opts.Buttons) > 0 {
		keyboard = s.buildKeyboard(opts.Buttons, opts.MiniAppURL)
	}

	sentCount := 0
	failedCount := 0

	for i, customer := range customers {
		sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)

		var sendErr error
		if opts != nil && opts.MediaFileID != "" {
			// Отправка с медиа
			sendErr = s.sendMediaMessage(sendCtx, customer.TelegramID, messageText, opts, keyboard)
		} else {
			// Отправка только текста
			params := &bot.SendMessageParams{
				ChatID:    customer.TelegramID,
				Text:      messageText,
				ParseMode: models.ParseModeHTML,
			}
			if keyboard != nil {
				params.ReplyMarkup = keyboard
			}
			_, sendErr = s.bot.SendMessage(sendCtx, params)
		}
		cancel()

		if sendErr != nil {
			failedCount++
		} else {
			sentCount++
		}

		// Обновляем прогресс каждые 100 сообщений
		if (i+1)%100 == 0 {
			_ = s.broadcastRepo.UpdateProgress(ctx, broadcastID, sentCount, failedCount)
			slog.Info("Broadcast progress", "id", broadcastID, "sent", sentCount, "failed", failedCount, "total", totalCount)
		}

		// Задержка 35ms между сообщениями (~28 msg/sec, лимит Telegram ~30 msg/sec)
		time.Sleep(35 * time.Millisecond)
	}

	// Финальное обновление
	status := string(database.BroadcastStatusCompleted)
	if failedCount > 0 {
		status = string(database.BroadcastStatusPartial)
	}

	err = s.broadcastRepo.UpdateStatus(ctx, broadcastID, status, sentCount, failedCount)
	if err != nil {
		return fmt.Errorf("failed to update final status: %w", err)
	}

	slog.Info("Broadcast completed",
		"id", utils.MaskHalfInt64(broadcastID),
		"sent", sentCount,
		"failed", failedCount,
		"total", totalCount,
	)

	return nil
}

// buildKeyboard создает inline клавиатуру из списка кнопок
// Используем префикс bc_ для broadcast кнопок чтобы отличать от обычных
func (s *BroadcastService) buildKeyboard(buttons []string, miniAppURL string) *models.InlineKeyboardMarkup {
	var rows [][]models.InlineKeyboardButton

	for _, btn := range buttons {
		switch strings.ToLower(btn) {
		case "promo":
			rows = append(rows, []models.InlineKeyboardButton{
				{Text: "🎟 Промокод", CallbackData: "bc_promo"},
			})
		case "subscription":
			if miniAppURL != "" {
				// Кнопка с mini app
				rows = append(rows, []models.InlineKeyboardButton{
					{Text: "🌐 Ваша подписка", WebApp: &models.WebAppInfo{URL: miniAppURL}},
				})
			} else {
				// Fallback на главное меню
				rows = append(rows, []models.InlineKeyboardButton{
					{Text: "🌐 Главное меню", CallbackData: "start"},
				})
			}
		case "buy":
			rows = append(rows, []models.InlineKeyboardButton{
				{Text: "🛒 Купить", CallbackData: "bc_buy"},
			})
		}
	}

	if len(rows) == 0 {
		return nil
	}

	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func (s *BroadcastService) getTargetCustomers(ctx context.Context, targetType string) ([]database.Customer, error) {
	switch targetType {
	case "all":
		return s.getAllCustomers(ctx)
	case "with_subscription":
		return s.getCustomersWithSubscription(ctx)
	case "without_subscription":
		return s.getCustomersWithoutSubscription(ctx)
	case "expiring":
		return s.getUsersWithExpiringSubscription(ctx)
	case "start_only":
		return s.customerRepository.FindStartOnlyCustomers(ctx)
	default:
		return nil, fmt.Errorf("unknown target type: %s", targetType)
	}
}

func (s *BroadcastService) getAllCustomers(ctx context.Context) ([]database.Customer, error) {
	return s.customerRepository.FindAll(ctx)
}

func (s *BroadcastService) getCustomersWithSubscription(ctx context.Context) ([]database.Customer, error) {
	customers, err := s.customerRepository.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	var result []database.Customer
	now := time.Now()
	for _, customer := range customers {
		if customer.ExpireAt != nil && customer.ExpireAt.After(now) {
			result = append(result, customer)
		}
	}

	return result, nil
}

func (s *BroadcastService) getCustomersWithoutSubscription(ctx context.Context) ([]database.Customer, error) {
	customers, err := s.customerRepository.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	var result []database.Customer
	now := time.Now()
	for _, customer := range customers {
		if customer.ExpireAt == nil || customer.ExpireAt.Before(now) {
			result = append(result, customer)
		}
	}

	return result, nil
}

func (s *BroadcastService) getUsersWithExpiringSubscription(ctx context.Context) ([]database.Customer, error) {
	now := time.Now()
	startDate := now
	endDate := now.Add(3 * 24 * time.Hour) // 3 дня

	customers, err := s.customerRepository.FindByExpirationRange(ctx, startDate, endDate)
	if err != nil {
		return nil, err
	}
	if customers == nil {
		return []database.Customer{}, nil
	}
	return *customers, nil
}

func (s *BroadcastService) GetBroadcastHistory(ctx context.Context, limit, offset int) ([]database.BroadcastHistory, error) {
	return s.broadcastRepo.List(ctx, limit, offset)
}

func (s *BroadcastService) GetBroadcast(ctx context.Context, id int64) (*database.BroadcastHistory, error) {
	return s.broadcastRepo.FindByID(ctx, id)
}

func (s *BroadcastService) DeleteBroadcast(ctx context.Context, id int64) error {
	return s.broadcastRepo.Delete(ctx, id)
}

// sendMediaMessage отправляет сообщение с медиа в зависимости от типа
func (s *BroadcastService) sendMediaMessage(ctx context.Context, chatID int64, caption string, opts *BroadcastOptions, keyboard *models.InlineKeyboardMarkup) error {
	switch opts.MediaType {
	case MediaTypePhoto:
		params := &bot.SendPhotoParams{
			ChatID:    chatID,
			Photo:     &models.InputFileString{Data: opts.MediaFileID},
			Caption:   caption,
			ParseMode: models.ParseModeHTML,
		}
		if keyboard != nil {
			params.ReplyMarkup = keyboard
		}
		_, err := s.bot.SendPhoto(ctx, params)
		return err

	case MediaTypeGIF:
		params := &bot.SendAnimationParams{
			ChatID:    chatID,
			Animation: &models.InputFileString{Data: opts.MediaFileID},
			Caption:   caption,
			ParseMode: models.ParseModeHTML,
		}
		if keyboard != nil {
			params.ReplyMarkup = keyboard
		}
		_, err := s.bot.SendAnimation(ctx, params)
		return err

	case MediaTypeVideo:
		params := &bot.SendVideoParams{
			ChatID:    chatID,
			Video:     &models.InputFileString{Data: opts.MediaFileID},
			Caption:   caption,
			ParseMode: models.ParseModeHTML,
		}
		if keyboard != nil {
			params.ReplyMarkup = keyboard
		}
		_, err := s.bot.SendVideo(ctx, params)
		return err

	case MediaTypeVideoNote:
		// VideoNote не поддерживает caption и кнопки
		_, err := s.bot.SendVideoNote(ctx, &bot.SendVideoNoteParams{
			ChatID:    chatID,
			VideoNote: &models.InputFileString{Data: opts.MediaFileID},
		})
		return err

	default:
		// Fallback на фото если тип не указан
		params := &bot.SendPhotoParams{
			ChatID:    chatID,
			Photo:     &models.InputFileString{Data: opts.MediaFileID},
			Caption:   caption,
			ParseMode: models.ParseModeHTML,
		}
		if keyboard != nil {
			params.ReplyMarkup = keyboard
		}
		_, err := s.bot.SendPhoto(ctx, params)
		return err
	}
}
