// Package postgres реализует хранилище реестра рейтинга в PostgreSQL.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
	"github.com/anton1ks96/mykct-api/internal/attendance/repository"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	"github.com/jmoiron/sqlx"
)

// Соответствие интерфейсу проверяется на этапе компиляции.
var _ repository.Leaderboard = (*LeaderboardRepository)(nil)

var log = logger.ComponentLogger("attendance.repository")

// cohortWarnSize - размер когорты, выше которого курс наверняка выведен неверно.
const cohortWarnSize = 1000

// participantColumns - колонки участника для выдачи наружу. streak_at в
// доменную модель не едет: он служебный, им только отсекается старый забор.
const participantColumns = `login, academic_group, course, current_streak, longest_streak,
	total_days_attended, attendance_rate, last_attended_date, empty_runs, inactive,
	registered_at, updated_at`

// participantRow - участник рейтинга в PostgreSQL. Логин лежит открытым: без
// него воркер не спросит портал, который узнаёт студента по cookie с логином.
// Наружу логин не уходит ни в каком виде.
type participantRow struct {
	Login             string    `db:"login"`
	AcademicGroup     string    `db:"academic_group"`
	Course            string    `db:"course"`
	CurrentStreak     int       `db:"current_streak"`
	LongestStreak     int       `db:"longest_streak"`
	TotalDaysAttended int       `db:"total_days_attended"`
	AttendanceRate    float64   `db:"attendance_rate"`
	LastAttendedDate  string    `db:"last_attended_date"`
	EmptyRuns         int       `db:"empty_runs"`
	Inactive          bool      `db:"inactive"`
	RegisteredAt      time.Time `db:"registered_at"`
	// UpdatedAt - когда участника в последний раз пытались пересчитать, удачно
	// или нет. NULL - ни разу, по нему строится очередь воркера
	UpdatedAt *time.Time `db:"updated_at"`
}

// toDomain переводит строку в доменную модель.
func (r *participantRow) toDomain() domain.Participant {
	participant := domain.Participant{
		Login:             r.Login,
		AcademicGroup:     r.AcademicGroup,
		Course:            r.Course,
		CurrentStreak:     r.CurrentStreak,
		LongestStreak:     r.LongestStreak,
		TotalDaysAttended: r.TotalDaysAttended,
		AttendanceRate:    r.AttendanceRate,
		LastAttendedDate:  r.LastAttendedDate,
		EmptyRuns:         r.EmptyRuns,
		Inactive:          r.Inactive,
		RegisteredAt:      r.RegisteredAt,
	}
	if r.UpdatedAt != nil {
		participant.UpdatedAt = *r.UpdatedAt
	}

	return participant
}

// LeaderboardRepository хранит реестр участников рейтинга в PostgreSQL.
type LeaderboardRepository struct {
	db *sqlx.DB
}

// NewLeaderboardRepository создаёт репозиторий реестра поверх клиента PostgreSQL.
func NewLeaderboardRepository(db *sqlx.DB) *LeaderboardRepository {
	return &LeaderboardRepository{db: db}
}

// Register заводит участника или освежает его группу. Отметку последнего
// пересчёта метод не двигает: иначе студент, регулярно открывающий приложение,
// навсегда выпал бы из очереди воркера.
func (r *LeaderboardRepository) Register(ctx context.Context, participant *domain.Participant) error {
	op := logger.NewLogOp(ctx, log, "Register")

	query := `
		INSERT INTO attendance_leaderboard (login, academic_group, course, registered_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (login) DO UPDATE SET
			academic_group = EXCLUDED.academic_group,
			course = EXCLUDED.course
		WHERE attendance_leaderboard.academic_group IS DISTINCT FROM EXCLUDED.academic_group
			OR attendance_leaderboard.course IS DISTINCT FROM EXCLUDED.course`
	_, err := r.db.ExecContext(ctx, query, participant.Login, participant.AcademicGroup,
		participant.Course, time.Now())
	if err != nil {
		op.Failed(err).Str("login", participant.Login).Msg("failed to register participant")
		return fmt.Errorf("failed to register leaderboard participant: %w", err)
	}

	op.Debug().Str("login", participant.Login).Msg("leaderboard participant registered")

	return nil
}

// SaveStreak записывает серию, забранную с портала в момент fetchedAt. Сравнение
// идёт по времени забора, а не записи: медленный ответ портала иначе затирал бы
// более свежие данные просто потому, что дошёл позже.
func (r *LeaderboardRepository) SaveStreak(
	ctx context.Context,
	login string,
	streak domain.Streak,
	fetchedAt time.Time,
) error {
	op := logger.NewLogOp(ctx, log, "SaveStreak")

	query := `
		UPDATE attendance_leaderboard
		SET current_streak = $3, longest_streak = $4, total_days_attended = $5,
			attendance_rate = $6, last_attended_date = $7,
			inactive = FALSE, empty_runs = 0, streak_at = $2, updated_at = $8
		WHERE login = $1 AND (streak_at IS NULL OR streak_at < $2)`
	res, err := r.db.ExecContext(ctx, query, login, fetchedAt, streak.CurrentStreak,
		streak.LongestStreak, streak.TotalDaysAttended, streak.AttendanceRate,
		streak.LastAttendedDate, time.Now())
	if err != nil {
		op.Failed(err).Str("login", login).Msg("failed to save participant streak")
		return fmt.Errorf("failed to save participant streak: %w", err)
	}

	if updated, _ := res.RowsAffected(); updated == 0 {
		op.Debug().Str("login", login).Msg("participant streak skipped, a fresher fetch is stored")
		return nil
	}

	op.Debug().Str("login", login).Msg("participant streak saved")

	return nil
}

// Reactivate снимает отметку простоя. Зовётся только на собственный заход
// студента: фоновый пересчёт воскрешать погасший логин не должен.
func (r *LeaderboardRepository) Reactivate(ctx context.Context, login string) error {
	op := logger.NewLogOp(ctx, log, "Reactivate")

	query := `UPDATE attendance_leaderboard SET inactive = FALSE, empty_runs = 0 WHERE login = $1`
	if _, err := r.db.ExecContext(ctx, query, login); err != nil {
		op.Failed(err).Str("login", login).Msg("failed to reactivate participant")
		return fmt.Errorf("failed to reactivate participant: %w", err)
	}

	op.Debug().Str("login", login).Msg("participant reactivated")

	return nil
}

// MarkAttempt отмечает неудачную попытку пересчёта. Серия остаётся прежней, но
// участник уходит в конец очереди: иначе логин, который портал не отдаёт
// никогда, вечно занимает её голову и не пускает остальных.
func (r *LeaderboardRepository) MarkAttempt(ctx context.Context, login string) error {
	op := logger.NewLogOp(ctx, log, "MarkAttempt")

	query := `UPDATE attendance_leaderboard SET updated_at = $2 WHERE login = $1`
	if _, err := r.db.ExecContext(ctx, query, login, time.Now()); err != nil {
		op.Failed(err).Str("login", login).Msg("failed to mark refresh attempt")
		return fmt.Errorf("failed to mark refresh attempt: %w", err)
	}

	return nil
}

// MarkEmpty засчитывает пустой ответ портала и гасит участника, когда их
// накопилось limit подряд: логин отчислен или сменился. Из реестра участник не
// удаляется, вернуть его в рейтинг может только собственный вход в приложение.
func (r *LeaderboardRepository) MarkEmpty(
	ctx context.Context,
	login string,
	fetchedAt time.Time,
	limit int,
) error {
	op := logger.NewLogOp(ctx, log, "MarkEmpty")

	query := `
		UPDATE attendance_leaderboard
		SET empty_runs = empty_runs + 1, updated_at = $3
		WHERE login = $1 AND (streak_at IS NULL OR streak_at < $2)`
	res, err := r.db.ExecContext(ctx, query, login, fetchedAt, time.Now())
	if err != nil {
		op.Failed(err).Str("login", login).Msg("failed to count empty portal answer")
		return fmt.Errorf("failed to count empty portal answer: %w", err)
	}
	if updated, _ := res.RowsAffected(); updated == 0 {
		op.Debug().Str("login", login).Msg("empty portal answer skipped, a fresher fetch is stored")
		return nil
	}

	query = `
		UPDATE attendance_leaderboard
		SET inactive = TRUE
		WHERE login = $1 AND (streak_at IS NULL OR streak_at < $2)
			AND empty_runs >= $3 AND inactive = FALSE`
	res, err = r.db.ExecContext(ctx, query, login, fetchedAt, limit)
	if err != nil {
		op.Failed(err).Str("login", login).Msg("failed to deactivate participant")
		return fmt.Errorf("failed to deactivate participant: %w", err)
	}

	if updated, _ := res.RowsAffected(); updated > 0 {
		op.Completed().Str("login", login).Msg("participant deactivated after empty portal answers")
	}

	return nil
}

// ByCourse возвращает живых участников курса. Лимита нет: курс - это несколько
// сотен строк, а его отсутствие снимает риск обрезать когорту на месте
// подсчёта мест.
func (r *LeaderboardRepository) ByCourse(ctx context.Context, course string) ([]domain.Participant, error) {
	op := logger.NewLogOp(ctx, log, "ByCourse")

	query := `SELECT ` + participantColumns + `
		FROM attendance_leaderboard
		WHERE course = $1 AND inactive = FALSE AND streak_at IS NOT NULL
		ORDER BY current_streak DESC`

	var rows []participantRow
	if err := r.db.SelectContext(ctx, &rows, query, course); err != nil {
		op.Failed(err).Str("course", course).Msg("failed to list course participants")
		return nil, fmt.Errorf("failed to list course participants: %w", err)
	}

	if len(rows) > cohortWarnSize {
		log.Warn().Str("course", course).Int("participants", len(rows)).
			Msg("leaderboard cohort is suspiciously large, check course extraction")
	}

	participants := make([]domain.Participant, 0, len(rows))
	for i := range rows {
		participants = append(participants, rows[i].toDomain())
	}

	op.Debug().Str("course", course).Int("participants", len(participants)).
		Msg("course participants listed")

	return participants, nil
}

// Stale возвращает участников, чью серию не пересчитывали дольше срока, начиная
// с самых давних. Так прогон воркера ограничен размером выборки и не зависит от
// размера реестра.
func (r *LeaderboardRepository) Stale(
	ctx context.Context,
	olderThan time.Time,
	limit int,
) ([]domain.Participant, error) {
	op := logger.NewLogOp(ctx, log, "Stale")

	query := `SELECT ` + participantColumns + `
		FROM attendance_leaderboard
		WHERE inactive = FALSE AND (updated_at IS NULL OR updated_at < $1)
		ORDER BY updated_at ASC NULLS FIRST
		LIMIT $2`

	var rows []participantRow
	if err := r.db.SelectContext(ctx, &rows, query, olderThan, limit); err != nil {
		op.Failed(err).Msg("failed to list stale participants")
		return nil, fmt.Errorf("failed to list stale participants: %w", err)
	}

	participants := make([]domain.Participant, 0, len(rows))
	for i := range rows {
		participants = append(participants, rows[i].toDomain())
	}

	op.Debug().Int("participants", len(participants)).Msg("stale participants listed")

	return participants, nil
}
