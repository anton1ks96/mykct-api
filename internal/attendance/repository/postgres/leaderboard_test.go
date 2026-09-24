package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
	"github.com/anton1ks96/mykct-api/internal/platform/pgtest"
)

// newLeaderboardRepo поднимает репозиторий на чистом реестре рейтинга.
func newLeaderboardRepo(t *testing.T) *LeaderboardRepository {
	t.Helper()

	return NewLeaderboardRepository(pgtest.DB(t, "attendance_leaderboard"))
}

// participant собирает участника реестра.
func participant(login, group, course string) *domain.Participant {
	return &domain.Participant{Login: login, AcademicGroup: group, Course: course}
}

// streak собирает посчитанную серию.
func streak(current, longest int) domain.Streak {
	return domain.Streak{
		CurrentStreak:     current,
		LongestStreak:     longest,
		TotalDaysAttended: 20,
		AttendanceRate:    0.8,
		LastAttendedDate:  "2026-09-21",
	}
}

// TestLeaderboardRegisterQueuesNewcomer - у нового участника пересчёта ещё не
// было, и он должен стоять в голове очереди воркера, а не в конце.
func TestLeaderboardRegisterQueuesNewcomer(t *testing.T) {
	repo := newLeaderboardRepo(t)
	ctx := context.Background()

	if err := repo.Register(ctx, participant("i24s0291", "ИТ25-11", "ИТ25")); err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}
	if err := repo.Register(ctx, participant("i24s0291", "ИТ25-12", "ИТ25")); err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}

	stale, err := repo.Stale(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("Stale() returned error: %v", err)
	}
	if len(stale) != 1 {
		t.Fatalf("Stale() = %+v, want one participant", stale)
	}
	if !stale[0].UpdatedAt.IsZero() {
		t.Errorf("Stale() returned updated_at %s, want zero for a newcomer", stale[0].UpdatedAt)
	}
	if stale[0].AcademicGroup != "ИТ25-12" {
		t.Errorf("Register() did not refresh the group: %q", stale[0].AcademicGroup)
	}
	if stale[0].RegisteredAt.IsZero() {
		t.Error("Register() left registered_at empty")
	}
}

// TestLeaderboardSaveStreak - серия попадает в выдачу курса, а более ранний
// забор не затирает более поздний: медленный ответ портала приходит позже.
func TestLeaderboardSaveStreak(t *testing.T) {
	repo := newLeaderboardRepo(t)
	ctx := context.Background()
	now := time.Now()

	for _, login := range []string{"i24s0291", "i24s0292"} {
		if err := repo.Register(ctx, participant(login, "ИТ25-11", "ИТ25")); err != nil {
			t.Fatalf("Register() returned error: %v", err)
		}
	}

	cohort, err := repo.ByCourse(ctx, "ИТ25")
	if err != nil {
		t.Fatalf("ByCourse() returned error: %v", err)
	}
	if len(cohort) != 0 {
		t.Errorf("ByCourse() = %+v, want nobody before the first fetch", cohort)
	}

	if err := repo.SaveStreak(ctx, "i24s0291", streak(5, 9), now); err != nil {
		t.Fatalf("SaveStreak() returned error: %v", err)
	}
	if err := repo.SaveStreak(ctx, "i24s0292", streak(7, 7), now); err != nil {
		t.Fatalf("SaveStreak() returned error: %v", err)
	}
	if err := repo.SaveStreak(ctx, "i24s0291", streak(1, 1), now.Add(-time.Hour)); err != nil {
		t.Fatalf("SaveStreak() of an older fetch returned error: %v", err)
	}

	cohort, err = repo.ByCourse(ctx, "ИТ25")
	if err != nil {
		t.Fatalf("ByCourse() returned error: %v", err)
	}
	if len(cohort) != 2 {
		t.Fatalf("ByCourse() = %+v, want two participants", cohort)
	}
	if cohort[0].Login != "i24s0292" || cohort[0].CurrentStreak != 7 {
		t.Errorf("ByCourse() = %+v, want the longest streak first", cohort[0])
	}
	if cohort[1].CurrentStreak != 5 {
		t.Errorf("SaveStreak() let an older fetch overwrite a newer one: %+v", cohort[1])
	}
	if cohort[1].LongestStreak != 9 || cohort[1].LastAttendedDate != "2026-09-21" {
		t.Errorf("ByCourse() lost the summary: %+v", cohort[1])
	}
}

// TestLeaderboardMarkEmpty - логин гаснет после предела пустых ответов подряд,
// из реестра не удаляется, а вернуть его в рейтинг может только свой вход.
func TestLeaderboardMarkEmpty(t *testing.T) {
	repo := newLeaderboardRepo(t)
	ctx := context.Background()
	now := time.Now()

	if err := repo.Register(ctx, participant("i24s0291", "ИТ25-11", "ИТ25")); err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}
	if err := repo.SaveStreak(ctx, "i24s0291", streak(5, 9), now); err != nil {
		t.Fatalf("SaveStreak() returned error: %v", err)
	}

	for i := 1; i <= 2; i++ {
		if err := repo.MarkEmpty(ctx, "i24s0291", now.Add(time.Duration(i)*time.Minute), 3); err != nil {
			t.Fatalf("MarkEmpty() returned error: %v", err)
		}
	}
	cohort, err := repo.ByCourse(ctx, "ИТ25")
	if err != nil {
		t.Fatalf("ByCourse() returned error: %v", err)
	}
	if len(cohort) != 1 {
		t.Errorf("MarkEmpty() dropped the participant before the limit: %+v", cohort)
	}

	if err := repo.MarkEmpty(ctx, "i24s0291", now.Add(3*time.Minute), 3); err != nil {
		t.Fatalf("MarkEmpty() returned error: %v", err)
	}
	cohort, err = repo.ByCourse(ctx, "ИТ25")
	if err != nil {
		t.Fatalf("ByCourse() returned error: %v", err)
	}
	if len(cohort) != 0 {
		t.Errorf("MarkEmpty() = %+v, want nobody after the limit", cohort)
	}
	stale, err := repo.Stale(ctx, now.Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("Stale() returned error: %v", err)
	}
	if len(stale) != 0 {
		t.Errorf("Stale() = %+v, want nobody: the login is out", stale)
	}

	if err := repo.MarkEmpty(ctx, "i24s0291", now.Add(-time.Hour), 3); err != nil {
		t.Fatalf("MarkEmpty() of an older fetch returned error: %v", err)
	}

	if err := repo.Reactivate(ctx, "i24s0291"); err != nil {
		t.Fatalf("Reactivate() returned error: %v", err)
	}
	cohort, err = repo.ByCourse(ctx, "ИТ25")
	if err != nil {
		t.Fatalf("ByCourse() returned error: %v", err)
	}
	if len(cohort) != 1 || cohort[0].EmptyRuns != 0 {
		t.Errorf("Reactivate() = %+v, want the participant back with a clean counter", cohort)
	}
}

// TestLeaderboardMarkAttempt - неудачная попытка уводит логин в конец очереди,
// иначе нефетчащийся логин навсегда занимает её голову.
func TestLeaderboardMarkAttempt(t *testing.T) {
	repo := newLeaderboardRepo(t)
	ctx := context.Background()

	if err := repo.Register(ctx, participant("i24s0291", "ИТ25-11", "ИТ25")); err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}
	if err := repo.MarkAttempt(ctx, "i24s0291"); err != nil {
		t.Fatalf("MarkAttempt() returned error: %v", err)
	}

	stale, err := repo.Stale(ctx, time.Now().Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("Stale() returned error: %v", err)
	}
	if len(stale) != 0 {
		t.Errorf("Stale() = %+v, want nobody: the attempt was just made", stale)
	}
}
