package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// GetAttendance возвращает занятия студента за период с отметками посещаемости.
func (s *Service) GetAttendance(ctx context.Context, input GetAttendanceInput) ([]domain.Record, error) {
	op := logger.NewLogOp(ctx, log, "GetAttendance")

	records, err := s.fetchAttendance(ctx, op, input.Login, input.Start, input.End)
	if err != nil {
		return nil, err
	}

	collapseSubGroups(records)

	op.Completed().Str("login", input.Login).Int("records", len(records)).
		Msg("attendance served from portal")

	return records, nil
}

// fetchAttendance забирает отметки у портала. Недоступный портал - ожидаемая
// деградация, а не сбой сервиса, поэтому warn: error-логи уезжают в Sentry.
func (s *Service) fetchAttendance(
	ctx context.Context,
	op *logger.LogOp,
	login, start, end string,
) ([]domain.Record, error) {
	records, err := s.portal.FetchAttendance(ctx, login, start, end)
	if err == nil {
		return records, nil
	}

	if errors.Is(err, domain.ErrPortalUnavailable) {
		op.Warn().Err(err).Str("login", login).Msg("college portal unavailable")
		return nil, err
	}

	op.Failed(err).Str("login", login).Msg("failed to fetch attendance")
	return nil, fmt.Errorf("failed to fetch attendance: %w", err)
}

// collapseSubGroups схлопывает одиночную подгруппу в само занятие, как в
// расписании. ClID заменяется на SClID: детали занятия портал держит на подгруппе.
func collapseSubGroups(records []domain.Record) {
	for i := range records {
		if len(records[i].SubGroup) != 1 {
			continue
		}

		sg := records[i].SubGroup[0]
		if sg.SClID != 0 {
			records[i].ClID = sg.SClID
		}
		records[i].Title = sg.STitle
		if records[i].Topic == "" {
			records[i].Topic = sg.STopic
		}
		if records[i].Room == "" {
			records[i].Room = sg.SCaID
		}
		records[i].SubGroup = nil
	}
}
