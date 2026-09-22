package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anton1ks96/mykct-api/internal/platform/pgtest"
	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
)

// newWeekStateRepo поднимает репозиторий на чистой таблице состояний недель.
func newWeekStateRepo(t *testing.T) *WeekStateRepository {
	t.Helper()

	return NewWeekStateRepository(pgtest.DB(t, "schedule_week_states"), time.Hour)
}

// weekState собирает состояние недели группы на момент опроса.
func weekState(group string, at time.Time) *domain.WeekState {
	return &domain.WeekState{
		Group:         group,
		WeekStart:     "2026-09-28",
		WeekEnd:       "2026-10-04",
		LastCheckedAt: at,
	}
}

// TestWeekStateCreateOnce - гонка инстансов упирается в первичный ключ, и
// второй заводящий должен получить ErrWeekStateExists, а не дубль.
func TestWeekStateCreateOnce(t *testing.T) {
	repo := newWeekStateRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.Create(ctx, weekState("ИТ25-11", now)); err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}
	if err := repo.Create(ctx, weekState("ИТ25-11", now)); !errors.Is(err, domain.ErrWeekStateExists) {
		t.Errorf("Create() twice = %v, want ErrWeekStateExists", err)
	}

	state, err := repo.Find(ctx, "ИТ25-11", "2026-09-28")
	if err != nil {
		t.Fatalf("Find() returned error: %v", err)
	}
	if state.Published || state.PublishedAt != nil || state.NotifiedAt != nil || state.EventsHash != "" {
		t.Errorf("Create() stored a dirty state: %+v", state)
	}

	if _, err := repo.Find(ctx, "ИТ25-11", "2026-10-05"); !errors.Is(err, domain.ErrWeekStateNotFound) {
		t.Errorf("Find() of an unknown week = %v, want ErrWeekStateNotFound", err)
	}
}

// TestWeekStateBaseline - базис меняет тот, кто от него же считал разницу:
// проигравший инстанс свою разницу выбрасывает. Статус недели занятия не тащит.
func TestWeekStateBaseline(t *testing.T) {
	repo := newWeekStateRepo(t)
	ctx := context.Background()

	if err := repo.Create(ctx, weekState("ИТ25-11", time.Now().UTC())); err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}

	won, err := repo.ReplaceBaseline(ctx, "ИТ25-11", "2026-09-28", "", "hash-1", sampleEvents())
	if err != nil {
		t.Fatalf("ReplaceBaseline() returned error: %v", err)
	}
	if !won {
		t.Fatal("ReplaceBaseline() from an empty hash did not seed the baseline")
	}

	won, err = repo.ReplaceBaseline(ctx, "ИТ25-11", "2026-09-28", "", "hash-2", sampleEvents())
	if err != nil {
		t.Fatalf("ReplaceBaseline() returned error: %v", err)
	}
	if won {
		t.Error("ReplaceBaseline() from a stale hash won the race")
	}

	won, err = repo.ReplaceBaseline(ctx, "ИТ25-11", "2026-09-28", "hash-1", "hash-2", sampleEvents()[:1])
	if err != nil {
		t.Fatalf("ReplaceBaseline() returned error: %v", err)
	}
	if !won {
		t.Error("ReplaceBaseline() from the current hash lost the race")
	}

	state, err := repo.Find(ctx, "ИТ25-11", "2026-09-28")
	if err != nil {
		t.Fatalf("Find() returned error: %v", err)
	}
	if len(state.Events) != 1 || state.EventsHash != "hash-2" {
		t.Errorf("Find() = %d events with hash %q, want 1 and hash-2", len(state.Events), state.EventsHash)
	}

	status, err := repo.FindStatus(ctx, "ИТ25-11", "2026-09-28")
	if err != nil {
		t.Fatalf("FindStatus() returned error: %v", err)
	}
	if status.Events != nil {
		t.Errorf("FindStatus() pulled the baseline: %d events", len(status.Events))
	}
	if status.EventsHash != "hash-2" {
		t.Errorf("FindStatus() lost the hash: %q", status.EventsHash)
	}
}

// TestWeekStatePublishAndNotify - и появление расписания, и рассылку по нему
// засчитывает ровно один вызов: на этом держится работа нескольких инстансов.
func TestWeekStatePublishAndNotify(t *testing.T) {
	repo := newWeekStateRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.Create(ctx, weekState("ИТ25-11", now)); err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}

	claimed, err := repo.MarkPublished(ctx, "ИТ25-11", "2026-09-28", 12, now)
	if err != nil {
		t.Fatalf("MarkPublished() returned error: %v", err)
	}
	if !claimed {
		t.Fatal("MarkPublished() was not counted")
	}
	claimed, err = repo.MarkPublished(ctx, "ИТ25-11", "2026-09-28", 12, now)
	if err != nil {
		t.Fatalf("MarkPublished() returned error: %v", err)
	}
	if claimed {
		t.Error("MarkPublished() was counted twice")
	}

	if err := repo.Touch(ctx, "ИТ25-11", "2026-09-28", 14, now.Add(time.Minute)); err != nil {
		t.Fatalf("Touch() returned error: %v", err)
	}
	if err := repo.Touch(ctx, "ИТ25-11", "2026-10-05", 1, now); err != nil {
		t.Errorf("Touch() of a missing week returned error: %v", err)
	}

	pending, err := repo.PendingPublished(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("PendingPublished() returned error: %v", err)
	}
	if len(pending) != 1 || pending[0].EventsCount != 14 || pending[0].Events != nil {
		t.Fatalf("PendingPublished() = %+v, want one week without a baseline", pending)
	}

	claimed, err = repo.MarkNotified(ctx, "ИТ25-11", "2026-09-28", now)
	if err != nil {
		t.Fatalf("MarkNotified() returned error: %v", err)
	}
	if !claimed {
		t.Fatal("MarkNotified() was not counted")
	}
	claimed, err = repo.MarkNotified(ctx, "ИТ25-11", "2026-09-28", now)
	if err != nil {
		t.Fatalf("MarkNotified() returned error: %v", err)
	}
	if claimed {
		t.Error("MarkNotified() was counted twice")
	}

	pending, err = repo.PendingPublished(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("PendingPublished() returned error: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("PendingPublished() = %+v, want nothing after the push", pending)
	}
}

// TestWeekStateDeleteExpired - недели, за которыми больше не следят, уносит чистка.
func TestWeekStateDeleteExpired(t *testing.T) {
	db := pgtest.DB(t, "schedule_week_states")
	repo := NewWeekStateRepository(db, time.Hour)
	stale := NewWeekStateRepository(db, -time.Hour)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.Create(ctx, weekState("ИТ25-11", now)); err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}
	expired := weekState("ИТ24-11", now)
	expired.WeekStart = "2026-09-07"
	if err := stale.Create(ctx, expired); err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}

	deleted, err := repo.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("DeleteExpired() returned error: %v", err)
	}
	if deleted != 1 {
		t.Errorf("DeleteExpired() = %d, want 1", deleted)
	}
	if left := countRows(t, db, "schedule_week_states"); left != 1 {
		t.Errorf("DeleteExpired() left %d week states, want 1", left)
	}
}
