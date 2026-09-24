package postgres

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/anton1ks96/mykct-api/internal/auth/domain"
	"github.com/anton1ks96/mykct-api/internal/platform/pgtest"
)

// newSessionRepo поднимает репозиторий на чистой таблице сессий.
func newSessionRepo(t *testing.T) *SessionRepository {
	t.Helper()

	return NewSessionRepository(pgtest.DB(t, "refresh_sessions"))
}

// session собирает сессию студента с заданным хэшем, группой и сроком.
func session(hash, userID, group string, expiresAt time.Time) *domain.RefreshSession {
	return &domain.RefreshSession{
		TokenHash:     hash,
		UserID:        userID,
		Username:      "Иванов И. И.",
		Role:          domain.RoleStudent,
		AcademicGroup: group,
		Profile:       "BE",
		Subgroup:      "Подгр1",
		EnglishGroup:  "B1.21",
		ExpiresAt:     expiresAt,
		CreatedAt:     time.Now().UTC(),
	}
}

// TestSessionSaveAndFind - сохранённая сессия находится по хэшу вместе со всем
// снимком профиля, протухшая не находится вовсе.
func TestSessionSaveAndFind(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.Save(ctx, session("live", "i24s0291", "ИТ25-11", now.Add(time.Hour))); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}
	if err := repo.Save(ctx, session("dead", "i24s0292", "ИТ25-12", now.Add(-time.Hour))); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	found, err := repo.FindByTokenHash(ctx, "live")
	if err != nil {
		t.Fatalf("FindByTokenHash() returned error: %v", err)
	}
	if found.UserID != "i24s0291" || found.EnglishGroup != "B1.21" || found.Profile != "BE" {
		t.Errorf("FindByTokenHash() lost the profile snapshot: %+v", found)
	}

	if _, err := repo.FindByTokenHash(ctx, "dead"); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Errorf("FindByTokenHash() on an expired session = %v, want ErrSessionNotFound", err)
	}
}

// TestSessionRotate - ротация отдаёт новое состояние и обесценивает старый
// хэш: повторный заход с ним не должен проходить.
func TestSessionRotate(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.Save(ctx, session("old", "i24s0291", "ИТ25-11", now.Add(time.Hour))); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	rotated, err := repo.Rotate(ctx, "old", "new", now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("Rotate() returned error: %v", err)
	}
	if rotated.TokenHash != "new" || !rotated.ExpiresAt.After(now.Add(time.Hour)) {
		t.Errorf("Rotate() returned stale state: %+v", rotated)
	}

	if _, err := repo.Rotate(ctx, "old", "newer", now.Add(2*time.Hour)); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Errorf("Rotate() with the old hash = %v, want ErrSessionNotFound", err)
	}
}

// TestSessionActiveGroups - группы живых сессий отдаются без повторов и без
// пробелов по краям, протухшие и преподаватели в выборку не идут.
func TestSessionActiveGroups(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	saved := []*domain.RefreshSession{
		session("a", "i24s0291", "ИТ25-11 ", now.Add(time.Hour)),
		session("b", "i24s0292", "ИТ25-11", now.Add(time.Hour)),
		session("c", "i24s0293", "ИТ25-12", now.Add(-time.Hour)),
	}
	teacher := session("d", "t001", "", now.Add(time.Hour))
	teacher.Role = domain.RoleTeacher
	saved = append(saved, teacher)

	for _, s := range saved {
		if err := repo.Save(ctx, s); err != nil {
			t.Fatalf("Save() returned error: %v", err)
		}
	}

	groups, err := repo.ActiveAcademicGroups(ctx)
	if err != nil {
		t.Fatalf("ActiveAcademicGroups() returned error: %v", err)
	}
	if len(groups) != 1 || groups[0] != "ИТ25-11" {
		t.Errorf("ActiveAcademicGroups() = %v, want [ИТ25-11]", groups)
	}

	students, err := repo.ActiveStudents(ctx)
	if err != nil {
		t.Fatalf("ActiveStudents() returned error: %v", err)
	}
	if len(students) != 2 {
		t.Fatalf("ActiveStudents() = %v, want 2 students", students)
	}
	for _, student := range students {
		if student.AcademicGroup != "ИТ25-11" {
			t.Errorf("ActiveStudents() left an untrimmed group: %q", student.AcademicGroup)
		}
	}
}

// TestSessionActiveStudentsTakesFreshestGroup - у студента сессия на каждом
// устройстве, и группу нужно брать из самой свежей: его могли перевести.
func TestSessionActiveStudentsTakesFreshestGroup(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.Save(ctx, session("older", "i24s0291", "ИТ24-11", now.Add(time.Hour))); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}
	if err := repo.Save(ctx, session("fresher", "i24s0291", "ИТ25-11", now.Add(2*time.Hour))); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	students, err := repo.ActiveStudents(ctx)
	if err != nil {
		t.Fatalf("ActiveStudents() returned error: %v", err)
	}
	if len(students) != 1 || students[0].AcademicGroup != "ИТ25-11" {
		t.Errorf("ActiveStudents() = %+v, want one student in ИТ25-11", students)
	}
}

// TestSessionRevoke - выход идемпотентен, а отзыв по пользователю уносит все
// его устройства и не задевает чужие сессии.
func TestSessionRevoke(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, s := range []*domain.RefreshSession{
		session("one", "i24s0291", "ИТ25-11", now.Add(time.Hour)),
		session("two", "i24s0291", "ИТ25-11", now.Add(time.Hour)),
		session("other", "i24s0292", "ИТ25-11", now.Add(time.Hour)),
	} {
		if err := repo.Save(ctx, s); err != nil {
			t.Fatalf("Save() returned error: %v", err)
		}
	}

	if err := repo.Revoke(ctx, "one"); err != nil {
		t.Fatalf("Revoke() returned error: %v", err)
	}
	if err := repo.Revoke(ctx, "unknown"); err != nil {
		t.Errorf("Revoke() of an unknown session returned error: %v", err)
	}
	if err := repo.RevokeAllByUser(ctx, "i24s0291"); err != nil {
		t.Fatalf("RevokeAllByUser() returned error: %v", err)
	}

	if _, err := repo.FindByTokenHash(ctx, "two"); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Errorf("RevokeAllByUser() left a session behind: %v", err)
	}
	if _, err := repo.FindByTokenHash(ctx, "other"); err != nil {
		t.Errorf("RevokeAllByUser() touched another user: %v", err)
	}
}

// TestSessionDeleteExpired - чистка уносит только протухшие сессии.
func TestSessionDeleteExpired(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.Save(ctx, session("live", "i24s0291", "ИТ25-11", now.Add(time.Hour))); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}
	if err := repo.Save(ctx, session("dead", "i24s0292", "ИТ25-12", now.Add(-time.Hour))); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	deleted, err := repo.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("DeleteExpired() returned error: %v", err)
	}
	if deleted != 1 {
		t.Errorf("DeleteExpired() = %d, want 1", deleted)
	}

	var hashes []string
	if err := repo.db.SelectContext(ctx, &hashes, `SELECT token_hash FROM refresh_sessions`); err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}
	if !slices.Equal(hashes, []string{"live"}) {
		t.Errorf("DeleteExpired() left %v, want [live]", hashes)
	}
}
