// Package service реализует юзкейсы push-уведомлений: учёт устройств и
// рассылку через Firebase Cloud Messaging.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"firebase.google.com/go/v4/messaging"
	"github.com/anton1ks96/mykct-api/internal/notification/domain"
	"github.com/anton1ks96/mykct-api/pkg/collegetime"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

var log = logger.ComponentLogger("notification.service")

// maxTokensPerBatch - предел FCM на один multicast-запрос.
const maxTokensPerBatch = 500

// AndroidChannelID - канал уведомлений на Android. Клиент заводит канал с тем
// же идентификатором, иначе система положит пуш в канал по умолчанию.
const AndroidChannelID = "schedule"

// ErrPushDisabled - рассылка выключена: не задан ключ сервисного аккаунта.
var ErrPushDisabled = errors.New("push notifications are disabled")

// Devices - хранилище устройств. Реализуется mongo.DeviceRepository.
type Devices interface {
	Upsert(ctx context.Context, device *domain.Device) error
	DeleteTokens(ctx context.Context, tokens []string) error
	TokensByGroup(ctx context.Context, group string) ([]string, error)
}

// Service учитывает устройства и рассылает на них уведомления.
type Service struct {
	devices Devices
	fcm     *messaging.Client // nil - рассылка выключена, устройства всё равно копятся
}

// NewService собирает сервис. fcm может быть nil: тогда регистрация работает, а
// NotifyGroup отвечает ErrPushDisabled.
func NewService(devices Devices, fcm *messaging.Client) *Service {
	return &Service{devices: devices, fcm: fcm}
}

// RegisterDeviceInput - устройство, которое прислал клиент, и его владелец из токена.
type RegisterDeviceInput struct {
	DeviceID      string
	Token         string
	Platform      string
	UserID        string
	AcademicGroup string
}

// RegisterDevice запоминает устройство за пользователем и его группой.
func (s *Service) RegisterDevice(ctx context.Context, input RegisterDeviceInput) error {
	return s.devices.Upsert(ctx, &domain.Device{
		DeviceID:      input.DeviceID,
		Token:         input.Token,
		Platform:      input.Platform,
		UserID:        input.UserID,
		AcademicGroup: input.AcademicGroup,
		UpdatedAt:     time.Now().UTC(),
	})
}

// UnregisterDevice забывает устройство по токену - при выходе из аккаунта.
func (s *Service) UnregisterDevice(ctx context.Context, token string) error {
	return s.devices.DeleteTokens(ctx, []string{token})
}

// quietFrom и quietTo - окно по времени колледжа, когда пуш приходит без
// звука: воркер расписания крутится круглосуточно, а будить группу ночью нельзя
const (
	quietFrom = 22
	quietTo   = 8
)

// apnsConfig задаёт звук уведомления. Общий notification FCM кладёт в aps.alert,
// а звук живёт только в самом aps, поэтому без этого блока баннер приходит молча.
// В тихие часы блок не отправляется вовсе - уведомление остаётся беззвучным.
func apnsConfig(now time.Time) *messaging.APNSConfig {
	hour := now.In(collegetime.TZ()).Hour()
	if hour >= quietFrom || hour < quietTo {
		return nil
	}

	return &messaging.APNSConfig{
		Payload: &messaging.APNSPayload{Aps: &messaging.Aps{Sound: "default"}},
	}
}

// NotifyGroup рассылает уведомление всем устройствам группы. Токены, которые
// FCM назвал мёртвыми, удаляются по ходу.
func (s *Service) NotifyGroup(ctx context.Context, group, title, body string, data map[string]string) error {
	if s.fcm == nil {
		return ErrPushDisabled
	}

	op := logger.NewLogOp(ctx, log, "NotifyGroup")

	tokens, err := s.devices.TokensByGroup(ctx, group)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		op.Debug().Str("group", group).Msg("no devices in group")
		return nil
	}

	var sent, failed int
	var dead []string
	for start := 0; start < len(tokens); start += maxTokensPerBatch {
		batch := tokens[start:min(start+maxTokensPerBatch, len(tokens))]

		resp, err := s.fcm.SendEachForMulticast(ctx, &messaging.MulticastMessage{
			Tokens:       batch,
			Notification: &messaging.Notification{Title: title, Body: body},
			Data:         data,
			Android: &messaging.AndroidConfig{
				Notification: &messaging.AndroidNotification{ChannelID: AndroidChannelID},
			},
			APNS: apnsConfig(time.Now()),
		})
		if err != nil {
			return fmt.Errorf("failed to send push to group %s: %w", group, err)
		}

		sent += resp.SuccessCount
		for i, r := range resp.Responses {
			if r.Success {
				continue
			}
			// InvalidArgument сюда не входит: его же FCM отвечает на кривой
			// payload, и тогда удаление снесло бы все токены группы разом
			if messaging.IsUnregistered(r.Error) || messaging.IsSenderIDMismatch(r.Error) {
				dead = append(dead, batch[i])
				continue
			}
			failed++
			op.Warn().Err(r.Error).Str("group", group).Msg("push was not delivered")
		}
	}

	if err := s.devices.DeleteTokens(ctx, dead); err != nil {
		op.Warn().Err(err).Msg("failed to drop dead tokens")
	}

	op.Info().Str("group", group).Int("sent", sent).Int("failed", failed).
		Int("dead", len(dead)).Msg("push sent")

	return nil
}
