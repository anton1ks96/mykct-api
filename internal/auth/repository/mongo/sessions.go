// Package mongo реализует хранилище refresh-сессий в MongoDB.
package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anton1ks96/mykct-api/internal/auth/domain"
	"github.com/anton1ks96/mykct-api/internal/auth/repository"
	"github.com/anton1ks96/mykct-api/pkg/database/mongodb"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Соответствие интерфейсам проверяется на этапе компиляции.
var (
	_ repository.SessionRepository = (*SessionRepository)(nil)
	_ mongodb.IndexEnsurer         = (*SessionRepository)(nil)
)

var log = logger.ComponentLogger("auth.repository")

// sessionsCollection - коллекция refresh-сессий.
const sessionsCollection = "refresh_sessions"

// sessionDoc - документ сессии в MongoDB. Доменная модель про bson не знает.
type sessionDoc struct {
	TokenHash     string    `bson:"token_hash"`
	UserID        string    `bson:"user_id"`
	Username      string    `bson:"username"`
	Role          string    `bson:"role"`
	AcademicGroup string    `bson:"academic_group,omitempty"`
	Profile       string    `bson:"profile,omitempty"`
	Subgroup      string    `bson:"subgroup,omitempty"`
	EnglishGroup  string    `bson:"english_group,omitempty"`
	ExpiresAt     time.Time `bson:"expires_at"`
	CreatedAt     time.Time `bson:"created_at"`
}

// toDomain переводит документ в доменную модель.
func (d *sessionDoc) toDomain() *domain.RefreshSession {
	return &domain.RefreshSession{
		TokenHash:     d.TokenHash,
		UserID:        d.UserID,
		Username:      d.Username,
		Role:          d.Role,
		AcademicGroup: d.AcademicGroup,
		Profile:       d.Profile,
		Subgroup:      d.Subgroup,
		EnglishGroup:  d.EnglishGroup,
		ExpiresAt:     d.ExpiresAt,
		CreatedAt:     d.CreatedAt,
	}
}

// fromDomain переводит доменную модель в документ.
func fromDomain(s *domain.RefreshSession) *sessionDoc {
	return &sessionDoc{
		TokenHash:     s.TokenHash,
		UserID:        s.UserID,
		Username:      s.Username,
		Role:          s.Role,
		AcademicGroup: s.AcademicGroup,
		Profile:       s.Profile,
		Subgroup:      s.Subgroup,
		EnglishGroup:  s.EnglishGroup,
		ExpiresAt:     s.ExpiresAt,
		CreatedAt:     s.CreatedAt,
	}
}

// SessionRepository хранит refresh-сессии в MongoDB.
type SessionRepository struct {
	coll *mongo.Collection
}

// NewSessionRepository создаёт репозиторий сессий поверх клиента MongoDB.
func NewSessionRepository(client *mongo.Client, database string) *SessionRepository {
	return &SessionRepository{coll: client.Database(database).Collection(sessionsCollection)}
}

// EnsureIndexes заводит индексы коллекции сессий.
func (r *SessionRepository) EnsureIndexes(ctx context.Context) error {
	op := logger.NewLogOp(ctx, log, "EnsureIndexes")

	models := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "token_hash", Value: 1}},
			Options: options.Index().SetName("token_hash_uniq").SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}},
			Options: options.Index().SetName("user_id_idx"),
		},
		{
			// Протухшие сессии Mongo удаляет сама, но с задержкой до минуты,
			// поэтому выборки всё равно фильтруют по expires_at
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetName("expires_at_ttl").SetExpireAfterSeconds(0),
		},
	}

	if _, err := r.coll.Indexes().CreateMany(ctx, models); err != nil {
		op.Failed(err).Msg("failed to create indexes")
		return fmt.Errorf("failed to create refresh session indexes: %w", err)
	}

	op.Completed().Msg("refresh session indexes ensured")

	return nil
}

// Save сохраняет новую сессию.
func (r *SessionRepository) Save(ctx context.Context, session *domain.RefreshSession) error {
	op := logger.NewLogOp(ctx, log, "Save")

	if _, err := r.coll.InsertOne(ctx, fromDomain(session)); err != nil {
		op.Failed(err).Str("user_id", session.UserID).Msg("failed to save refresh session")
		return fmt.Errorf("failed to save refresh session: %w", err)
	}

	op.Debug().Str("user_id", session.UserID).Msg("refresh session saved")

	return nil
}

// FindByTokenHash возвращает живую сессию по хэшу токена.
func (r *SessionRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*domain.RefreshSession, error) {
	op := logger.NewLogOp(ctx, log, "FindByTokenHash")

	var doc sessionDoc
	err := r.coll.FindOne(ctx, aliveFilter(tokenHash)).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			op.Debug().Msg("refresh session not found")
			return nil, domain.ErrSessionNotFound
		}
		op.Failed(err).Msg("failed to find refresh session")
		return nil, fmt.Errorf("failed to find refresh session: %w", err)
	}

	op.Debug().Str("user_id", doc.UserID).Msg("refresh session found")

	return doc.toDomain(), nil
}

// Rotate атомарно подменяет хэш и срок живой сессии одной операцией: старый
// токен перестаёт действовать ровно в тот момент, когда начинает новый.
func (r *SessionRepository) Rotate(
	ctx context.Context,
	oldHash, newHash string,
	expiresAt time.Time,
) (*domain.RefreshSession, error) {
	op := logger.NewLogOp(ctx, log, "Rotate")

	update := bson.M{"$set": bson.M{"token_hash": newHash, "expires_at": expiresAt}}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)

	var doc sessionDoc
	err := r.coll.FindOneAndUpdate(ctx, aliveFilter(oldHash), update, opts).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			op.Debug().Msg("refresh session not found or already rotated")
			return nil, domain.ErrSessionNotFound
		}
		op.Failed(err).Msg("failed to rotate refresh session")
		return nil, fmt.Errorf("failed to rotate refresh session: %w", err)
	}

	op.Debug().Str("user_id", doc.UserID).Msg("refresh session rotated")

	return doc.toDomain(), nil
}

// Revoke удаляет сессию. Отсутствие сессии ошибкой не считается: выход должен
// быть идемпотентным.
func (r *SessionRepository) Revoke(ctx context.Context, tokenHash string) error {
	op := logger.NewLogOp(ctx, log, "Revoke")

	res, err := r.coll.DeleteOne(ctx, bson.M{"token_hash": tokenHash})
	if err != nil {
		op.Failed(err).Msg("failed to revoke refresh session")
		return fmt.Errorf("failed to revoke refresh session: %w", err)
	}

	op.Debug().Int64("deleted", res.DeletedCount).Msg("refresh session revoked")

	return nil
}

// RevokeAllByUser удаляет все сессии пользователя.
func (r *SessionRepository) RevokeAllByUser(ctx context.Context, userID string) error {
	op := logger.NewLogOp(ctx, log, "RevokeAllByUser")

	res, err := r.coll.DeleteMany(ctx, bson.M{"user_id": userID})
	if err != nil {
		op.Failed(err).Str("user_id", userID).Msg("failed to revoke user sessions")
		return fmt.Errorf("failed to revoke user sessions: %w", err)
	}

	op.Completed().Str("user_id", userID).Int64("deleted", res.DeletedCount).
		Msg("user sessions revoked")

	return nil
}

// aliveFilter отбирает сессию по хэшу токена, отсекая протухшие.
func aliveFilter(tokenHash string) bson.M {
	return bson.M{
		"token_hash": tokenHash,
		"expires_at": bson.M{"$gt": time.Now()},
	}
}
