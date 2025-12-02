package notification

import (
	"context"
	"strconv"

	"remnawave-tg-shop-bot/internal/remnawave"
)

// RemnawaveClientAdapter адаптирует remnawave.Client к интерфейсу remnawaveClient
type RemnawaveClientAdapter struct {
	client *remnawave.Client
}

// NewRemnawaveClientAdapter создаёт новый адаптер для remnawave.Client
func NewRemnawaveClientAdapter(client *remnawave.Client) *RemnawaveClientAdapter {
	return &RemnawaveClientAdapter{client: client}
}

// GetUserByTelegramID получает информацию о пользователе по Telegram ID
func (a *RemnawaveClientAdapter) GetUserByTelegramID(ctx context.Context, telegramID int64) (*RemnawaveUserInfo, error) {
	info, err := a.client.GetUserByTelegramID(ctx, telegramID)
	if err != nil {
		return nil, err
	}

	return &RemnawaveUserInfo{
		UUID:             info.UUID,
		Username:         info.Username,
		FirstConnectedAt: info.FirstConnectedAt,
		ExpireAt:         info.ExpireAt,
		Status:           info.Status,
	}, nil
}

// telegramIDToString конвертирует telegram ID в строку
func telegramIDToString(id int64) string {
	return strconv.FormatInt(id, 10)
}
