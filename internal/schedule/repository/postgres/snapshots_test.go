package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anton1ks96/mykct-api/internal/platform/pgtest"
	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/jmoiron/sqlx"
)

// snapshotTables - таблицы снимков портала.
var snapshotTables = []string{"schedule_snapshots", "class_details_snapshots"}

// sampleEvents - пара занятий, одно с подгруппами: так отвечает портал.
func sampleEvents() []domain.Event {
	return []domain.Event{
		{
			ClID: "101", Day: "2026-09-21", Group: "ИТ25-11", Start: "09:00", End: "10:30",
			Room: "301", Title: "Программирование", Color: "#FF0000",
			SubGroup: []domain.SubGroup{
				{SClID: "101-1", SGrID: "Подгр1", SGCaID: "301", STitle: "Программирование"},
				{SClID: "101-2", SGrID: "Подгр2", SGCaID: "302", STitle: "Базы данных"},
			},
		},
		{ClID: "102", Day: "2026-09-22", Group: "ИТ25-11", Start: "10:40", End: "12:10", Title: "Математика"},
	}
}

// snapshot собирает снимок расписания группы за неделю.
func snapshot(group string, events []domain.Event, fetchedAt time.Time) *domain.Snapshot {
	return &domain.Snapshot{
		Group:     group,
		Start:     "2026-09-21",
		End:       "2026-09-27",
		Events:    events,
		FetchedAt: fetchedAt,
	}
}

// TestSnapshotScheduleRoundTrip - занятия с подгруппами возвращаются такими
// же, а повторное сохранение затирает снимок за тот же период.
func TestSnapshotScheduleRoundTrip(t *testing.T) {
	repo := NewSnapshotRepository(pgtest.DB(t, snapshotTables...), time.Hour)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.SaveSchedule(ctx, snapshot("ИТ25-11", sampleEvents(), now)); err != nil {
		t.Fatalf("SaveSchedule() returned error: %v", err)
	}

	found, err := repo.FindSchedule(ctx, "ИТ25-11", "2026-09-21", "2026-09-27")
	if err != nil {
		t.Fatalf("FindSchedule() returned error: %v", err)
	}
	if len(found.Events) != 2 {
		t.Fatalf("FindSchedule() returned %d events, want 2", len(found.Events))
	}
	if len(found.Events[0].SubGroup) != 2 || found.Events[0].SubGroup[1].STitle != "Базы данных" {
		t.Errorf("FindSchedule() lost subgroups: %+v", found.Events[0].SubGroup)
	}
	if found.Events[1].Title != "Математика" {
		t.Errorf("FindSchedule() lost an event: %+v", found.Events[1])
	}

	if err := repo.SaveSchedule(ctx, snapshot("ИТ25-11", sampleEvents()[:1], now.Add(time.Minute))); err != nil {
		t.Fatalf("SaveSchedule() returned error: %v", err)
	}
	found, err = repo.FindSchedule(ctx, "ИТ25-11", "2026-09-21", "2026-09-27")
	if err != nil {
		t.Fatalf("FindSchedule() returned error: %v", err)
	}
	if len(found.Events) != 1 {
		t.Errorf("SaveSchedule() did not overwrite the period: %d events", len(found.Events))
	}
}

// TestSnapshotKeepsNilAndEmpty - нулевой слайс должен вернуться нулевым, а
// пустой пустым: на этом держится ответ клиенту по пустому периоду.
func TestSnapshotKeepsNilAndEmpty(t *testing.T) {
	repo := NewSnapshotRepository(pgtest.DB(t, snapshotTables...), time.Hour)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.SaveSchedule(ctx, snapshot("ИТ25-12", nil, now)); err != nil {
		t.Fatalf("SaveSchedule() returned error: %v", err)
	}
	if err := repo.SaveSchedule(ctx, snapshot("ИТ25-13", []domain.Event{}, now)); err != nil {
		t.Fatalf("SaveSchedule() returned error: %v", err)
	}

	nilSnap, err := repo.FindSchedule(ctx, "ИТ25-12", "2026-09-21", "2026-09-27")
	if err != nil {
		t.Fatalf("FindSchedule() returned error: %v", err)
	}
	if nilSnap.Events != nil {
		t.Errorf("FindSchedule() turned nil events into %+v", nilSnap.Events)
	}

	emptySnap, err := repo.FindSchedule(ctx, "ИТ25-13", "2026-09-21", "2026-09-27")
	if err != nil {
		t.Fatalf("FindSchedule() returned error: %v", err)
	}
	if emptySnap.Events == nil || len(emptySnap.Events) != 0 {
		t.Errorf("FindSchedule() turned empty events into %+v", emptySnap.Events)
	}
}

// TestSnapshotAcceptsNULByte - портал отдаёт произвольный текст, а JSONB не
// принимает нулевой байт: запись всё равно должна проходить.
func TestSnapshotAcceptsNULByte(t *testing.T) {
	repo := NewSnapshotRepository(pgtest.DB(t, snapshotTables...), time.Hour)
	ctx := context.Background()
	now := time.Now().UTC()

	events := []domain.Event{{ClID: "101", Title: "Програм\x00мирование", Day: "2026-09-21"}}
	if err := repo.SaveSchedule(ctx, snapshot("ИТ25-11", events, now)); err != nil {
		t.Fatalf("SaveSchedule() with a NUL byte returned error: %v", err)
	}

	found, err := repo.FindSchedule(ctx, "ИТ25-11", "2026-09-21", "2026-09-27")
	if err != nil {
		t.Fatalf("FindSchedule() returned error: %v", err)
	}
	if found.Events[0].Title != "Программирование" {
		t.Errorf("FindSchedule() = %q, want the title without the NUL byte", found.Events[0].Title)
	}

	details := &domain.ClassDetails{
		ClID:      "101",
		Details:   map[string]any{"teacher": "Сидоро\x00ва"},
		FetchedAt: now,
	}
	if err := repo.SaveClassDetails(ctx, details); err != nil {
		t.Fatalf("SaveClassDetails() with a NUL byte returned error: %v", err)
	}
}

// TestSnapshotClassDetails - тело портала проносится как есть, неизвестное
// занятие отдаёт доменную ошибку.
func TestSnapshotClassDetails(t *testing.T) {
	repo := NewSnapshotRepository(pgtest.DB(t, snapshotTables...), time.Hour)
	ctx := context.Background()
	now := time.Now().UTC()

	err := repo.SaveClassDetails(ctx, &domain.ClassDetails{
		ClID:      "101",
		Details:   map[string]any{"teacher": "Сидорова С. С.", "hours": 2.0},
		FetchedAt: now,
	})
	if err != nil {
		t.Fatalf("SaveClassDetails() returned error: %v", err)
	}

	found, err := repo.FindClassDetails(ctx, "101")
	if err != nil {
		t.Fatalf("FindClassDetails() returned error: %v", err)
	}
	if found.Details["teacher"] != "Сидорова С. С." || found.Details["hours"] != 2.0 {
		t.Errorf("FindClassDetails() = %+v, want the portal body as is", found.Details)
	}

	if _, err := repo.FindClassDetails(ctx, "999"); !errors.Is(err, domain.ErrClassDetailsUnavailable) {
		t.Errorf("FindClassDetails() of an unknown class = %v, want ErrClassDetailsUnavailable", err)
	}
}

// TestSnapshotExpired - протухший снимок не отдаётся, хотя строка ещё на месте:
// чистка приходит с задержкой, а срок кэша точный. Её прогон уносит обе таблицы.
func TestSnapshotExpired(t *testing.T) {
	db := pgtest.DB(t, snapshotTables...)
	repo := NewSnapshotRepository(db, time.Hour)
	stale := NewSnapshotRepository(db, -time.Hour)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := stale.SaveSchedule(ctx, snapshot("ИТ25-11", sampleEvents(), now)); err != nil {
		t.Fatalf("SaveSchedule() returned error: %v", err)
	}
	err := stale.SaveClassDetails(ctx, &domain.ClassDetails{
		ClID: "101", Details: map[string]any{"a": "b"}, FetchedAt: now,
	})
	if err != nil {
		t.Fatalf("SaveClassDetails() returned error: %v", err)
	}

	if _, err := repo.FindSchedule(ctx, "ИТ25-11", "2026-09-21", "2026-09-27"); !errors.Is(err, domain.ErrScheduleUnavailable) {
		t.Errorf("FindSchedule() of an expired snapshot = %v, want ErrScheduleUnavailable", err)
	}
	if _, err := repo.FindClassDetails(ctx, "101"); !errors.Is(err, domain.ErrClassDetailsUnavailable) {
		t.Errorf("FindClassDetails() of expired details = %v, want ErrClassDetailsUnavailable", err)
	}

	deleted, err := repo.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("DeleteExpired() returned error: %v", err)
	}
	if deleted != 2 {
		t.Errorf("DeleteExpired() = %d, want 2", deleted)
	}
	if left := countRows(t, db, "schedule_snapshots"); left != 0 {
		t.Errorf("DeleteExpired() left %d snapshots", left)
	}
}

// countRows считает строки таблицы - проверка того, что чистка дошла до базы.
func countRows(t *testing.T, db *sqlx.DB, table string) int {
	t.Helper()

	var count int
	if err := db.Get(&count, `SELECT count(*) FROM `+table); err != nil {
		t.Fatalf("failed to count rows in %s: %v", table, err)
	}

	return count
}
