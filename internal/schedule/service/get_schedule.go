package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// GetSchedule возвращает расписание группы за период. Пока портал колледжа
// отвечает, данные берутся у него и попутно сохраняются; как только он падает,
// отдаётся последний снимок, а источник и его возраст уезжают в ответ.
func (s *Service) GetSchedule(ctx context.Context, input GetScheduleInput) (*GetScheduleOutput, error) {
	op := logger.NewLogOp(ctx, log, "GetSchedule")

	// 1. Портал спрашивается со всеми подгруппами: снимок зависит только от
	// группы и периода, а отбор нужной подгруппы делается ниже
	events, err := s.portal.FetchSchedule(ctx, input.Group, input.Start, input.End)
	if err == nil {
		fetchedAt := time.Now().UTC()
		s.saveSchedule(ctx, op, input, events, fetchedAt)

		op.Completed().Str("group", input.Group).Str("source", domain.SourceLive).
			Int("events", len(events)).Msg("schedule served from portal")

		return &GetScheduleOutput{
			Events:    selectEvents(events, input),
			Source:    domain.SourceLive,
			FetchedAt: fetchedAt,
		}, nil
	}

	if !errors.Is(err, domain.ErrPortalUnavailable) {
		op.Failed(err).Str("group", input.Group).Msg("failed to fetch schedule")
		return nil, fmt.Errorf("failed to fetch schedule: %w", err)
	}

	// 2. Портал недоступен - это ожидаемая деградация, а не сбой сервиса,
	// поэтому warn: error-логи уезжают в Sentry
	op.Warn().Err(err).Str("group", input.Group).
		Msg("college portal unavailable, falling back to cached schedule")

	snapshot, err := s.snapshots.FindSchedule(ctx, input.Group, input.Start, input.End)
	if err != nil {
		if errors.Is(err, domain.ErrScheduleUnavailable) {
			op.Warn().Str("group", input.Group).Msg("no cached schedule to fall back to")
			return nil, domain.ErrScheduleUnavailable
		}
		op.Failed(err).Str("group", input.Group).Msg("failed to read cached schedule")
		return nil, fmt.Errorf("failed to read cached schedule: %w", err)
	}

	op.Completed().Str("group", input.Group).Str("source", domain.SourceCache).
		Time("fetched_at", snapshot.FetchedAt).Int("events", len(snapshot.Events)).
		Msg("schedule served from cache")

	return &GetScheduleOutput{
		Events:    selectEvents(snapshot.Events, input),
		Source:    domain.SourceCache,
		FetchedAt: snapshot.FetchedAt,
	}, nil
}

// saveSchedule обновляет снимок расписания. Неудачная запись не должна ронять
// запрос: пользователю уже есть что показать.
func (s *Service) saveSchedule(
	ctx context.Context,
	op *logger.LogOp,
	input GetScheduleInput,
	events []domain.Event,
	fetchedAt time.Time,
) {
	snapshot := &domain.Snapshot{
		Group:     input.Group,
		Start:     input.Start,
		End:       input.End,
		Events:    events,
		FetchedAt: fetchedAt,
	}

	if err := s.snapshots.SaveSchedule(ctx, snapshot); err != nil {
		op.Warn().Err(err).Str("group", input.Group).Msg("failed to cache schedule snapshot")
	}
}
