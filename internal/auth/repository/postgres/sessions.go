// Package postgres реализует хранилище refresh-сессий в PostgreSQL.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/anton1ks96/mykct-api/internal/auth/domain"
	"github.com/anton1ks96/mykct-api/internal/auth/repository"
	pgdb "github.com/anton1ks96/mykct-api/pkg/database/postgres"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	"github.com/jmoiron/sqlx"
)

// Соответствие интерфейсам проверяется на этапе компиляции.
var (
	_ repository.SessionRepository = (*SessionRepository)(nil)
	_ pgdb.ExpiryCleaner           = (*SessionRepository)(nil)
)

var log = logger.ComponentLogger("auth.repository")

// sessionColumns - общий список колонок сессии для SELECT.
const sessionColumns = `token_hash, user_id, username, role, academic_group,
	profile, subgroup, english_group, expires_at, created_at`

// sessionRow - строка таблицы refresh_sessions. Доменная модель про db не знает.
type sessionRow struct {
	TokenHash     string    `db:"token_hash"`
	UserID        string    `db:"user_id"`
	Username      string    `db:"username"`
	Role          string    `db:"role"`
	AcademicGroup string    `db:"academic_group"`
	Profile       string    `db:"profile"`
	Subgroup      string    `db:"subgroup"`
	EnglishGroup  string    `db:"english_group"`
	ExpiresAt     time.Time `db:"expires_at"`
	CreatedAt     time.Time `db:"created_at"`
}

// toDomain переводит строку в доменную модель.
func (r *sessionRow) toDomain() *domain.RefreshSession {
	return &domain.RefreshSession{
		TokenHash:     r.TokenHash,
		UserID:        r.UserID,
		Username:      r.Username,
		Role:          r.Role,
		AcademicGroup: r.AcademicGroup,
		Profile:       r.Profile,
		Subgroup:      r.Subgroup,
		EnglishGroup:  r.EnglishGroup,
		ExpiresAt:     r.ExpiresAt,
		CreatedAt:     r.CreatedAt,
	}
}

// activeStudentRow - строка выборки активных студентов.
type activeStudentRow struct {
	UserID        string `db:"user_id"`
	AcademicGroup string `db:"academic_group"`
}

// SessionRepository хранит refresh-сессии в PostgreSQL.
type SessionRepository struct {
	db *sqlx.DB
}

// NewSessionRepository создаёт репозиторий сессий поверх клиента PostgreSQL.
func NewSessionRepository(db *sqlx.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

// Save сохраняет новую сессию.
func (r *SessionRepository) Save(ctx context.Context, session *domain.RefreshSession) error {
	op := logger.NewLogOp(ctx, log, "Save")

	query := `
		INSERT INTO refresh_sessions (token_hash, user_id, username, role, academic_group,
			profile, subgroup, english_group, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	_, err := r.db.ExecContext(ctx, query, session.TokenHash, session.UserID, session.Username,
		session.Role, session.AcademicGroup, session.Profile, session.Subgroup,
		session.EnglishGroup, session.ExpiresAt, session.CreatedAt)
	if err != nil {
		op.Failed(err).Str("user_id", session.UserID).Msg("failed to save refresh session")
		return fmt.Errorf("failed to save refresh session: %w", err)
	}

	op.Debug().Str("user_id", session.UserID).Msg("refresh session saved")

	return nil
}

// FindByTokenHash возвращает живую сессию по хэшу токена.
func (r *SessionRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*domain.RefreshSession, error) {
	op := logger.NewLogOp(ctx, log, "FindByTokenHash")

	query := `SELECT ` + sessionColumns + `
		FROM refresh_sessions
		WHERE token_hash = $1 AND expires_at > $2`

	var row sessionRow
	if err := r.db.GetContext(ctx, &row, query, tokenHash, time.Now()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			op.Debug().Msg("refresh session not found")
			return nil, domain.ErrSessionNotFound
		}
		op.Failed(err).Msg("failed to find refresh session")
		return nil, fmt.Errorf("failed to find refresh session: %w", err)
	}

	op.Debug().Str("user_id", row.UserID).Msg("refresh session found")

	return row.toDomain(), nil
}

// Rotate атомарно подменяет хэш и срок живой сессии одним запросом: старый
// токен перестаёт действовать ровно в тот момент, когда начинает новый.
func (r *SessionRepository) Rotate(
	ctx context.Context,
	oldHash, newHash string,
	expiresAt time.Time,
) (*domain.RefreshSession, error) {
	op := logger.NewLogOp(ctx, log, "Rotate")

	query := `
		UPDATE refresh_sessions
		SET token_hash = $1, expires_at = $2
		WHERE token_hash = $3 AND expires_at > $4
		RETURNING ` + sessionColumns

	var row sessionRow
	if err := r.db.GetContext(ctx, &row, query, newHash, expiresAt, oldHash, time.Now()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			op.Debug().Msg("refresh session not found or already rotated")
			return nil, domain.ErrSessionNotFound
		}
		op.Failed(err).Msg("failed to rotate refresh session")
		return nil, fmt.Errorf("failed to rotate refresh session: %w", err)
	}

	op.Debug().Str("user_id", row.UserID).Msg("refresh session rotated")

	return row.toDomain(), nil
}

// Revoke удаляет сессию. Отсутствие сессии ошибкой не считается: выход должен
// быть идемпотентным.
func (r *SessionRepository) Revoke(ctx context.Context, tokenHash string) error {
	op := logger.NewLogOp(ctx, log, "Revoke")

	res, err := r.db.ExecContext(ctx, `DELETE FROM refresh_sessions WHERE token_hash = $1`, tokenHash)
	if err != nil {
		op.Failed(err).Msg("failed to revoke refresh session")
		return fmt.Errorf("failed to revoke refresh session: %w", err)
	}

	deleted, _ := res.RowsAffected()
	op.Debug().Int64("deleted", deleted).Msg("refresh session revoked")

	return nil
}

// RevokeAllByUser удаляет все сессии пользователя.
func (r *SessionRepository) RevokeAllByUser(ctx context.Context, userID string) error {
	op := logger.NewLogOp(ctx, log, "RevokeAllByUser")

	res, err := r.db.ExecContext(ctx, `DELETE FROM refresh_sessions WHERE user_id = $1`, userID)
	if err != nil {
		op.Failed(err).Str("user_id", userID).Msg("failed to revoke user sessions")
		return fmt.Errorf("failed to revoke user sessions: %w", err)
	}

	deleted, _ := res.RowsAffected()
	op.Completed().Str("user_id", userID).Int64("deleted", deleted).Msg("user sessions revoked")

	return nil
}

// ActiveAcademicGroups возвращает академические группы живых сессий без повторов:
// по ним модуль расписания понимает, за кем вообще имеет смысл следить.
// Повторы убираются после обрезки: "ИТ25-11" и "ИТ25-11 " в каталоге дали бы
// группу дважды.
func (r *SessionRepository) ActiveAcademicGroups(ctx context.Context) ([]string, error) {
	op := logger.NewLogOp(ctx, log, "ActiveAcademicGroups")

	query := `
		SELECT DISTINCT btrim(academic_group)
		FROM refresh_sessions
		WHERE btrim(academic_group) <> '' AND expires_at > $1`

	var groups []string
	if err := r.db.SelectContext(ctx, &groups, query, time.Now()); err != nil {
		op.Failed(err).Msg("failed to list active academic groups")
		return nil, fmt.Errorf("failed to list active academic groups: %w", err)
	}

	op.Debug().Int("groups", len(groups)).Msg("active academic groups listed")

	return groups, nil
}

// ActiveStudents возвращает студентов с живыми сессиями без повторов: по ним
// другие модули монолита узнают, кто вообще пользуется приложением.
func (r *SessionRepository) ActiveStudents(ctx context.Context) ([]domain.ActiveStudent, error) {
	op := logger.NewLogOp(ctx, log, "ActiveStudents")

	query := `
		SELECT DISTINCT ON (user_id) user_id, btrim(academic_group) AS academic_group
		FROM refresh_sessions
		WHERE role = $1 AND user_id <> '' AND btrim(academic_group) <> '' AND expires_at > $2
		ORDER BY user_id, expires_at DESC`

	var rows []activeStudentRow
	if err := r.db.SelectContext(ctx, &rows, query, domain.RoleStudent, time.Now()); err != nil {
		op.Failed(err).Msg("failed to list active students")
		return nil, fmt.Errorf("failed to list active students: %w", err)
	}

	students := make([]domain.ActiveStudent, 0, len(rows))
	for _, row := range rows {
		students = append(students, domain.ActiveStudent{
			UserID:        row.UserID,
			AcademicGroup: row.AcademicGroup,
		})
	}

	op.Debug().Int("students", len(students)).Msg("active students listed")

	return students, nil
}

// DeleteExpired удаляет протухшие сессии. Выборки всё равно фильтруют по
// expires_at, поэтому чистка только освобождает место.
func (r *SessionRepository) DeleteExpired(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM refresh_sessions WHERE expires_at <= $1`, time.Now())
	if err != nil {
		logger.NewLogOp(ctx, log, "DeleteExpired").Failed(err).
			Msg("failed to delete expired refresh sessions")
		return 0, fmt.Errorf("failed to delete expired refresh sessions: %w", err)
	}

	deleted, _ := res.RowsAffected()

	return deleted, nil
}
