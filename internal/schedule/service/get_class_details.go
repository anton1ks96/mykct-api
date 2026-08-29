package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// GetClassDetails возвращает детали занятия. Схема ответа портала не
// фиксирована, поэтому тело проносится как есть; при недоступном портале
// отдаётся последний сохранённый снимок.
func (s *Service) GetClassDetails(ctx context.Context, clid string) (*GetClassDetailsOutput, error) {
	op := logger.NewLogOp(ctx, log, "GetClassDetails")

	details, err := s.portal.FetchClassDetails(ctx, clid)
	if err == nil {
		fetchedAt := time.Now().UTC()
		s.saveClassDetails(ctx, op, clid, details, fetchedAt)

		op.Completed().Str("clid", clid).Str("source", domain.SourceLive).
			Msg("class details served from portal")

		return &GetClassDetailsOutput{
			Details:   details,
			Source:    domain.SourceLive,
			FetchedAt: fetchedAt,
		}, nil
	}

	if !errors.Is(err, domain.ErrPortalUnavailable) {
		op.Failed(err).Str("clid", clid).Msg("failed to fetch class details")
		return nil, fmt.Errorf("failed to fetch class details: %w", err)
	}

	op.Warn().Err(err).Str("clid", clid).
		Msg("college portal unavailable, falling back to cached class details")

	cached, err := s.snapshots.FindClassDetails(ctx, clid)
	if err != nil {
		if errors.Is(err, domain.ErrClassDetailsUnavailable) {
			op.Warn().Str("clid", clid).Msg("no cached class details to fall back to")
			return nil, domain.ErrClassDetailsUnavailable
		}
		op.Failed(err).Str("clid", clid).Msg("failed to read cached class details")
		return nil, fmt.Errorf("failed to read cached class details: %w", err)
	}

	op.Completed().Str("clid", clid).Str("source", domain.SourceCache).
		Time("fetched_at", cached.FetchedAt).Msg("class details served from cache")

	return &GetClassDetailsOutput{
		Details:   cached.Details,
		Source:    domain.SourceCache,
		FetchedAt: cached.FetchedAt,
	}, nil
}

// saveClassDetails обновляет снимок деталей занятия, не роняя запрос при ошибке.
func (s *Service) saveClassDetails(
	ctx context.Context,
	op *logger.LogOp,
	clid string,
	details map[string]any,
	fetchedAt time.Time,
) {
	snapshot := &domain.ClassDetails{
		ClID:      clid,
		Details:   details,
		FetchedAt: fetchedAt,
	}

	if err := s.snapshots.SaveClassDetails(ctx, snapshot); err != nil {
		op.Warn().Err(err).Str("clid", clid).Msg("failed to cache class details snapshot")
	}
}
