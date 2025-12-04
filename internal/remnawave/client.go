package remnawave

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/utils"
	"strconv"
	"strings"
	"time"

	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"
	"github.com/google/uuid"
)

type Client struct {
	client *remapi.ClientExt
}

type headerTransport struct {
	base    http.RoundTripper
	local   bool
	headers map[string]string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())

	if t.local {
		r.Header.Set("x-forwarded-for", "127.0.0.1")
		r.Header.Set("x-forwarded-proto", "https")
	}

	for key, value := range t.headers {
		r.Header.Set(key, value)
	}

	return t.base.RoundTrip(r)
}

func NewClient(baseURL, token, mode string) *Client {
	local := mode == "local"
	headers := config.RemnawaveHeaders()

	client := &http.Client{
		Transport: &headerTransport{
			base:    http.DefaultTransport,
			local:   local,
			headers: headers,
		},
	}

	api, err := remapi.NewClient(baseURL, remapi.StaticToken{Token: token}, remapi.WithClient(client))
	if err != nil {
		panic(err)
	}
	return &Client{client: remapi.NewClientExt(api)}
}

func (r *Client) Ping(ctx context.Context) error {
	params := remapi.UsersControllerGetAllUsersParams{
		Size:  remapi.NewOptFloat64(1),
		Start: remapi.NewOptFloat64(0),
	}
	_, err := r.client.UsersControllerGetAllUsers(ctx, params)
	return err
}

// UserInfo содержит информацию о пользователе из Remnawave API
type UserInfo struct {
	UUID             uuid.UUID
	Username         string
	FirstConnectedAt *time.Time
	ExpireAt         time.Time
	Status           string
}

// GetUserByUUID получает пользователя по UUID (subscription link) для проверки firstConnectedAt
// Используется для определения, подключался ли триальный пользователь к сервису
func (r *Client) GetUserByUUID(ctx context.Context, userUUID uuid.UUID) (*UserInfo, error) {
	resp, err := r.client.UsersControllerGetUserByUuid(ctx, remapi.UsersControllerGetUserByUuidParams{UUID: userUUID.String()})
	if err != nil {
		return nil, err
	}

	switch v := resp.(type) {
	case *remapi.UsersControllerGetUserByUuidNotFound:
		return nil, errors.New("user not found")
	case *remapi.UserResponse:
		user := v.GetResponse()
		info := &UserInfo{
			UUID:     user.UUID,
			Username: user.Username,
			ExpireAt: user.ExpireAt,
			Status:   string(user.Status.Value),
		}
		// Проверяем firstConnectedAt
		if firstConnected, ok := user.FirstConnectedAt.Get(); ok {
			info.FirstConnectedAt = &firstConnected
		}
		return info, nil
	default:
		return nil, errors.New("unknown response type")
	}
}

// GetUserByTelegramID получает пользователя по Telegram ID для проверки firstConnectedAt
// Используется для определения, подключался ли триальный пользователь к сервису
func (r *Client) GetUserByTelegramID(ctx context.Context, telegramID int64) (*UserInfo, error) {
	resp, err := r.client.UsersControllerGetUserByTelegramId(ctx, remapi.UsersControllerGetUserByTelegramIdParams{
		TelegramId: strconv.FormatInt(telegramID, 10),
	})
	if err != nil {
		return nil, err
	}

	switch v := resp.(type) {
	case *remapi.UsersControllerGetUserByTelegramIdNotFound:
		return nil, errors.New("user not found")
	case *remapi.UsersResponse:
		users := v.GetResponse()
		if len(users) == 0 {
			return nil, errors.New("user not found")
		}
		// Берём первого пользователя (или ищем по username с telegram_id)
		var user *remapi.UsersResponseResponseItem
		for i := range users {
			if strings.Contains(users[i].Username, fmt.Sprintf("_%d", telegramID)) {
				user = &users[i]
				break
			}
		}
		if user == nil {
			user = &users[0]
		}

		info := &UserInfo{
			UUID:     user.UUID,
			Username: user.Username,
			ExpireAt: user.ExpireAt,
			Status:   string(user.Status.Value),
		}
		// Проверяем firstConnectedAt
		if firstConnected, ok := user.FirstConnectedAt.Get(); ok {
			info.FirstConnectedAt = &firstConnected
		}
		return info, nil
	default:
		return nil, errors.New("unknown response type")
	}
}

func (r *Client) GetUsers(ctx context.Context) (*[]remapi.GetAllUsersResponseDtoResponseUsersItem, error) {
	pager := remapi.NewPaginationHelper(250)
	users := make([]remapi.GetAllUsersResponseDtoResponseUsersItem, 0)

	for {
		params := remapi.UsersControllerGetAllUsersParams{
			Start: remapi.NewOptFloat64(float64(pager.Offset)),
			Size:  remapi.NewOptFloat64(float64(pager.Limit)),
		}

		resp, err := r.client.Users().GetAllUsers(ctx, params)
		if err != nil {
			return nil, err
		}

		response := resp.(*remapi.GetAllUsersResponseDto).GetResponse()
		users = append(users, response.Users...)

		if len(response.Users) < pager.Limit {
			break
		}

		if !pager.NextPage() {
			break
		}
	}

	return &users, nil
}

func (r *Client) DecreaseSubscription(ctx context.Context, telegramId int64, trafficLimit, days int) (*time.Time, error) {
	resp, err := r.client.Users().GetUserByTelegramId(ctx, remapi.UsersControllerGetUserByTelegramIdParams{TelegramId: strconv.FormatInt(telegramId, 10)})
	if err != nil {
		return nil, err
	}

	switch v := resp.(type) {
	case *remapi.UsersControllerGetUserByTelegramIdNotFound:
		return nil, errors.New("user in remnawave not found")
	case *remapi.UsersResponse:
		var existingUser *remapi.UsersResponseResponseItem
		for _, panelUser := range v.GetResponse() {
			if strings.Contains(panelUser.Username, fmt.Sprintf("_%d", telegramId)) {
				existingUser = &panelUser
			}
		}
		if existingUser == nil {
			existingUser = &v.GetResponse()[0]
		}
		updatedUser, err := r.updateUser(ctx, existingUser, trafficLimit, days)
		return &updatedUser.ExpireAt, err
	default:
		return nil, errors.New("unknown response type")
	}
}

func (r *Client) CreateOrUpdateUser(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, days int, isTrialUser bool) (*remapi.UserResponseResponse, error) {
	return r.CreateOrUpdateUserWithDeviceLimit(ctx, customerId, telegramId, trafficLimit, days, isTrialUser, nil)
}

// CreateOrUpdateUserWithDeviceLimit создаёт или обновляет пользователя с указанным лимитом устройств.
// deviceLimit - лимит устройств из выбранного тарифа (nil = не устанавливать)
func (r *Client) CreateOrUpdateUserWithDeviceLimit(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, days int, isTrialUser bool, deviceLimit *int) (*remapi.UserResponseResponse, error) {
	resp, err := r.client.UsersControllerGetUserByTelegramId(ctx, remapi.UsersControllerGetUserByTelegramIdParams{TelegramId: strconv.FormatInt(telegramId, 10)})
	if err != nil {
		return nil, err
	}

	switch v := resp.(type) {

	case *remapi.UsersControllerGetUserByTelegramIdNotFound:
		return r.createUserWithDeviceLimit(ctx, customerId, telegramId, trafficLimit, days, isTrialUser, deviceLimit)
	case *remapi.UsersResponse:
		var existingUser *remapi.UsersResponseResponseItem
		for _, panelUser := range v.GetResponse() {
			if strings.Contains(panelUser.Username, fmt.Sprintf("_%d", telegramId)) {
				existingUser = &panelUser
			}
		}
		if existingUser == nil {
			existingUser = &v.GetResponse()[0]
		}
		return r.updateUserWithDeviceLimit(ctx, existingUser, trafficLimit, days, deviceLimit)
	default:
		return nil, errors.New("unknown response type")
	}
}

func (r *Client) updateUser(ctx context.Context, existingUser *remapi.UsersResponseResponseItem, trafficLimit int, days int) (*remapi.UserResponseResponse, error) {
	return r.updateUserWithDeviceLimit(ctx, existingUser, trafficLimit, days, nil)
}

// updateUserWithDeviceLimit обновляет пользователя с опциональным лимитом устройств
func (r *Client) updateUserWithDeviceLimit(ctx context.Context, existingUser *remapi.UsersResponseResponseItem, trafficLimit int, days int, deviceLimit *int) (*remapi.UserResponseResponse, error) {

	newExpire := getNewExpire(days, existingUser.ExpireAt)

	resp, err := r.client.InternalSquadControllerGetInternalSquads(ctx)
	if err != nil {
		return nil, err
	}

	squads := resp.(*remapi.GetInternalSquadsResponseDto).GetResponse()

	selectedSquads := config.SquadUUIDs()

	squadId := make([]uuid.UUID, 0, len(selectedSquads))
	for _, squad := range squads.GetInternalSquads() {
		if selectedSquads != nil && len(selectedSquads) > 0 {
			if _, isExist := selectedSquads[squad.UUID]; !isExist {
				continue
			} else {
				squadId = append(squadId, squad.UUID)
			}
		} else {
			squadId = append(squadId, squad.UUID)
		}
	}

	userUpdate := &remapi.UpdateUserRequestDto{
		UUID:                 remapi.NewOptUUID(existingUser.UUID),
		ExpireAt:             remapi.NewOptDateTime(newExpire),
		Status:               remapi.NewOptUpdateUserRequestDtoStatus(remapi.UpdateUserRequestDtoStatusACTIVE),
		TrafficLimitBytes:    remapi.NewOptInt(trafficLimit),
		ActiveInternalSquads: squadId,
		TrafficLimitStrategy: remapi.NewOptUpdateUserRequestDtoTrafficLimitStrategy(getUpdateStrategy(config.TrafficLimitResetStrategy())),
	}

	// Применяем лимит устройств если указан тариф
	// Простая логика: пользователь получает то, за что платит
	// Если лимит отключен в панели (Null=true) → не трогаем
	if deviceLimit != nil {
		var currentLimit *int
		if !existingUser.HwidDeviceLimit.Null {
			val := existingUser.HwidDeviceLimit.Value
			currentLimit = &val
		}

		finalLimit := ResolveDeviceLimit(currentLimit, *deviceLimit)

		if finalLimit != nil {
			userUpdate.HwidDeviceLimit = remapi.NewOptNilInt(*finalLimit)
			slog.Debug("Setting device limit", "currentLimit", currentLimit, "tariffLimit", *deviceLimit, "finalLimit", *finalLimit)
		}
	}

	externalSquad := config.ExternalSquadUUID()
	if externalSquad != uuid.Nil {
		userUpdate.ExternalSquadUuid = remapi.NewOptNilUUID(externalSquad)
	}

	tag := config.RemnawaveTag()
	if tag != "" {
		userUpdate.Tag = remapi.NewOptNilString(tag)
	}

	var username string
	if ctx.Value("username") != nil {
		username = ctx.Value("username").(string)
		userUpdate.Description = remapi.NewOptNilString(username)
	} else {
		username = ""
	}

	updateUser, err := r.client.UsersControllerUpdateUser(ctx, userUpdate)
	if err != nil {
		return nil, err
	}
	if value, ok := updateUser.(*remapi.UsersControllerUpdateUserInternalServerError); ok {
		return nil, errors.New("error while updating user. message: " + value.GetMessage().Value + ". code: " + value.GetErrorCode().Value)
	}

	tgid, _ := existingUser.TelegramId.Get()
	slog.Info("updated user", "telegramId", utils.MaskHalf(strconv.Itoa(tgid)), "username", utils.MaskHalf(username), "days", days)
	return &updateUser.(*remapi.UserResponse).Response, nil
}

func (r *Client) createUser(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, days int, isTrialUser bool) (*remapi.UserResponseResponse, error) {
	return r.createUserWithDeviceLimit(ctx, customerId, telegramId, trafficLimit, days, isTrialUser, nil)
}

// createUserWithDeviceLimit создаёт нового пользователя с опциональным лимитом устройств
func (r *Client) createUserWithDeviceLimit(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, days int, isTrialUser bool, deviceLimit *int) (*remapi.UserResponseResponse, error) {
	expireAt := time.Now().UTC().AddDate(0, 0, days)
	username := generateUsername(customerId, telegramId)

	resp, err := r.client.InternalSquadControllerGetInternalSquads(ctx)
	if err != nil {
		return nil, err
	}

	squads := resp.(*remapi.GetInternalSquadsResponseDto).GetResponse()

	selectedSquads := config.SquadUUIDs()
	if isTrialUser {
		selectedSquads = config.TrialInternalSquads()
	}

	squadId := make([]uuid.UUID, 0, len(selectedSquads))
	for _, squad := range squads.GetInternalSquads() {
		if selectedSquads != nil && len(selectedSquads) > 0 {
			if _, isExist := selectedSquads[squad.UUID]; !isExist {
				continue
			} else {
				squadId = append(squadId, squad.UUID)
			}
		} else {
			squadId = append(squadId, squad.UUID)
		}
	}

	externalSquad := config.ExternalSquadUUID()
	if isTrialUser {
		externalSquad = config.TrialExternalSquadUUID()
	}

	strategy := config.TrafficLimitResetStrategy()
	if isTrialUser {
		strategy = config.TrialTrafficLimitResetStrategy()
	}

	createUserRequestDto := remapi.CreateUserRequestDto{
		Username:             username,
		ActiveInternalSquads: squadId,
		Status:               remapi.NewOptCreateUserRequestDtoStatus(remapi.CreateUserRequestDtoStatusACTIVE),
		TelegramId:           remapi.NewOptNilInt(int(telegramId)),
		ExpireAt:             expireAt,
		TrafficLimitStrategy: remapi.NewOptCreateUserRequestDtoTrafficLimitStrategy(getCreateStrategy(strategy)),
		TrafficLimitBytes:    remapi.NewOptInt(trafficLimit),
	}

	// Устанавливаем лимит устройств для нового пользователя (если указан тариф и не триал)
	if deviceLimit != nil && !isTrialUser {
		createUserRequestDto.HwidDeviceLimit = remapi.NewOptInt(*deviceLimit)
		slog.Debug("Setting device limit for new user", "deviceLimit", *deviceLimit)
	}

	if externalSquad != uuid.Nil {
		createUserRequestDto.ExternalSquadUuid = remapi.NewOptNilUUID(externalSquad)
	}
	tag := config.RemnawaveTag()
	if isTrialUser {
		tag = config.TrialRemnawaveTag()
	}
	if tag != "" {
		createUserRequestDto.Tag = remapi.NewOptNilString(tag)
	}

	var tgUsername string
	if ctx.Value("username") != nil {
		tgUsername = ctx.Value("username").(string)
		createUserRequestDto.Description = remapi.NewOptString(ctx.Value("username").(string))
	} else {
		tgUsername = ""
	}

	userCreate, err := r.client.UsersControllerCreateUser(ctx, &createUserRequestDto)
	if err != nil {
		return nil, err
	}
	slog.Info("created user", "telegramId", utils.MaskHalf(strconv.FormatInt(telegramId, 10)), "username", utils.MaskHalf(tgUsername), "days", days)
	return &userCreate.(*remapi.UserResponse).Response, nil
}

func generateUsername(customerId int64, telegramId int64) string {
	return fmt.Sprintf("%d_%d", customerId, telegramId)
}

func getNewExpire(daysToAdd int, currentExpire time.Time) time.Time {
	if daysToAdd <= 0 {
		if currentExpire.AddDate(0, 0, daysToAdd).Before(time.Now()) {
			return time.Now().UTC().AddDate(0, 0, 1)
		} else {
			return currentExpire.AddDate(0, 0, daysToAdd)
		}
	}

	if currentExpire.Before(time.Now().UTC()) || currentExpire.IsZero() {
		return time.Now().UTC().AddDate(0, 0, daysToAdd)
	}

	return currentExpire.AddDate(0, 0, daysToAdd)
}

func getCreateStrategy(s string) remapi.CreateUserRequestDtoTrafficLimitStrategy {
	switch s {
	case "DAY":
		return remapi.CreateUserRequestDtoTrafficLimitStrategyDAY
	case "WEEK":
		return remapi.CreateUserRequestDtoTrafficLimitStrategyWEEK
	case "NO_RESET":
		return remapi.CreateUserRequestDtoTrafficLimitStrategyNORESET
	default:
		return remapi.CreateUserRequestDtoTrafficLimitStrategyMONTH
	}
}

func getUpdateStrategy(s string) remapi.UpdateUserRequestDtoTrafficLimitStrategy {
	switch s {
	case "DAY":
		return remapi.UpdateUserRequestDtoTrafficLimitStrategyDAY
	case "WEEK":
		return remapi.UpdateUserRequestDtoTrafficLimitStrategyWEEK
	case "NO_RESET":
		return remapi.UpdateUserRequestDtoTrafficLimitStrategyNORESET
	default:
		return remapi.UpdateUserRequestDtoTrafficLimitStrategyMONTH
	}
}

// ResolveDeviceLimit определяет финальный лимит устройств при продлении подписки.
// Простая логика: пользователь получает то, за что платит.
// currentLimit передаётся только если у пользователя есть персональный лимит (Null=false).
// Если currentLimit == nil (лимит отключен в панели) → не устанавливаем.
func ResolveDeviceLimit(currentLimit *int, tariffLimit int) *int {
	// Если лимит отключен в панели (Null=true) → не трогаем
	if currentLimit == nil {
		return nil
	}

	// Есть персональный лимит → заменяем на новый тариф
	return &tariffLimit
}
