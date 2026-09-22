package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/anton1ks96/mykct-api/internal/platform/pgtest"
	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
)

// TestChangeSaveAndPending - разница восстанавливается целиком, включая
// подгруппы и разошедшиеся поля, и отдаётся от старых к новым.
func TestChangeSaveAndPending(t *testing.T) {
	repo := NewChangeRepository(pgtest.DB(t, "schedule_changes"), time.Hour)
	ctx := context.Background()
	now := time.Now().UTC()
	events := sampleEvents()

	err := repo.Save(ctx, &domain.WeekChanges{
		Group: "ИТ25-11", WeekStart: "2026-09-21", DetectedAt: now,
		Changes: []domain.EventChange{
			{Kind: domain.ChangeAdded, After: &events[0]},
			{Kind: domain.ChangeRemoved, Before: &events[1]},
			{
				Kind:   domain.ChangeChanged,
				Fields: []string{"room", "start"},
				Before: &events[0],
				After:  &events[1],
			},
		},
	})
	if err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	err = repo.Save(ctx, &domain.WeekChanges{
		Group: "ИТ24-11", WeekStart: "2026-09-21", DetectedAt: now.Add(time.Minute),
		Changes: []domain.EventChange{{Kind: domain.ChangeAdded, After: &events[0]}},
	})
	if err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	pending, err := repo.Pending(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("Pending() returned error: %v", err)
	}
	if len(pending) != 2 || pending[0].Group != "ИТ25-11" || pending[1].Group != "ИТ24-11" {
		t.Fatalf("Pending() = %+v, want oldest first", pending)
	}

	first := pending[0]
	if len(first.Changes) != 3 {
		t.Fatalf("Pending() returned %d changes, want 3", len(first.Changes))
	}
	added := first.Changes[0]
	if added.Before != nil || added.After == nil || added.After.ClID != "101" {
		t.Errorf("Pending() broke the added change: %+v", added)
	}
	if len(added.After.SubGroup) != 2 {
		t.Errorf("Pending() lost subgroups: %+v", added.After.SubGroup)
	}
	removed := first.Changes[1]
	if removed.After != nil || removed.Before == nil || removed.Before.ClID != "102" {
		t.Errorf("Pending() broke the removed change: %+v", removed)
	}
	if len(first.Changes[2].Fields) != 2 || first.Changes[2].Fields[1] != "start" {
		t.Errorf("Pending() lost the changed fields: %+v", first.Changes[2].Fields)
	}

	empty, err := repo.Pending(ctx, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Pending() returned error: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("Pending() with a fresh since = %+v, want nothing", empty)
	}
}

// TestChangeMarkNotifiedOnce - рассылкой владеет ровно один вызов, а кривой
// идентификатор до базы не доходит.
func TestChangeMarkNotifiedOnce(t *testing.T) {
	repo := NewChangeRepository(pgtest.DB(t, "schedule_changes"), time.Hour)
	ctx := context.Background()
	now := time.Now().UTC()
	events := sampleEvents()

	err := repo.Save(ctx, &domain.WeekChanges{
		Group: "ИТ25-11", WeekStart: "2026-09-21", DetectedAt: now,
		Changes: []domain.EventChange{{Kind: domain.ChangeAdded, After: &events[0]}},
	})
	if err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	pending, err := repo.Pending(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("Pending() returned error: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("Pending() returned %d changes, want 1", len(pending))
	}

	claimed, err := repo.MarkNotified(ctx, pending[0].ID, now)
	if err != nil {
		t.Fatalf("MarkNotified() returned error: %v", err)
	}
	if !claimed {
		t.Fatal("MarkNotified() was not counted")
	}
	claimed, err = repo.MarkNotified(ctx, pending[0].ID, now)
	if err != nil {
		t.Fatalf("MarkNotified() returned error: %v", err)
	}
	if claimed {
		t.Error("MarkNotified() was counted twice")
	}

	if _, err := repo.MarkNotified(ctx, "не-число", now); err == nil {
		t.Error("MarkNotified() accepted a malformed id")
	}

	left, err := repo.Pending(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("Pending() returned error: %v", err)
	}
	if len(left) != 0 {
		t.Errorf("Pending() = %+v, want nothing after the push", left)
	}
}

// TestChangeDeleteExpired - разницы, которые уже никому не нужны, уносит чистка.
func TestChangeDeleteExpired(t *testing.T) {
	db := pgtest.DB(t, "schedule_changes")
	repo := NewChangeRepository(db, time.Hour)
	stale := NewChangeRepository(db, -time.Hour)
	ctx := context.Background()
	now := time.Now().UTC()
	events := sampleEvents()

	fresh := &domain.WeekChanges{
		Group: "ИТ25-11", WeekStart: "2026-09-21", DetectedAt: now,
		Changes: []domain.EventChange{{Kind: domain.ChangeAdded, After: &events[0]}},
	}
	if err := repo.Save(ctx, fresh); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}
	if err := stale.Save(ctx, fresh); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	deleted, err := repo.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("DeleteExpired() returned error: %v", err)
	}
	if deleted != 1 {
		t.Errorf("DeleteExpired() = %d, want 1", deleted)
	}
	if left := countRows(t, db, "schedule_changes"); left != 1 {
		t.Errorf("DeleteExpired() left %d changes, want 1", left)
	}
}

// TestTrackedGroups - отметка о слежении заводится один раз и не сдвигается
// повторным вызовом: по ней новая группа отличается от смены недели у знакомой.
func TestTrackedGroups(t *testing.T) {
	db := pgtest.DB(t, "schedule_tracked_groups")
	repo := NewTrackedGroupRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.Track(ctx, "ИТ25-11", now); err != nil {
		t.Fatalf("Track() returned error: %v", err)
	}
	if err := repo.Track(ctx, "ИТ25-11", now.Add(time.Hour)); err != nil {
		t.Fatalf("Track() twice returned error: %v", err)
	}
	if err := repo.Track(ctx, "ИТ24-11", now); err != nil {
		t.Fatalf("Track() returned error: %v", err)
	}

	groups, err := repo.All(ctx)
	if err != nil {
		t.Fatalf("All() returned error: %v", err)
	}
	if len(groups) != 2 {
		t.Errorf("All() = %v, want two groups", groups)
	}

	var since time.Time
	err = db.GetContext(ctx, &since,
		`SELECT tracked_since FROM schedule_tracked_groups WHERE group_name = $1`, "ИТ25-11")
	if err != nil {
		t.Fatalf("failed to read tracked_since: %v", err)
	}
	if since.Sub(now).Abs() > time.Second {
		t.Errorf("Track() moved tracked_since to %s, want %s", since, now)
	}
}
