// Package repository объявляет интерфейсы хранилищ модуля аутентификации.
package repository

import (
	"context"
	"time"

	"github.com/anton1ks96/mykct-api/internal/auth/domain"
)

// UserDirectory - каталог пользователей колледжа. Пароль проверяется самим
// каталогом, поэтому он передаётся в каждый метод.
type UserDirectory interface {
	// Authenticate проверяет пару логин-пароль.
	Authenticate(ctx context.Context, userID, password string) error
	// GetByID возвращает учётную запись с определённой ролью.
	GetByID(ctx context.Context, userID, password string) (*domain.User, error)
	// GetUserGroups возвращает учебные группы пользователя.
	GetUserGroups(ctx context.Context, userID, password string) (*domain.UserGroups, error)
}

// SessionRepository - хранилище refresh-сессий. Сессия адресуется хэшем токена,
// сам токен хранилищу не передаётся.
type SessionRepository interface {
	// Save сохраняет новую сессию.
	Save(ctx context.Context, session *domain.RefreshSession) error
	// FindByTokenHash возвращает живую сессию по хэшу токена.
	FindByTokenHash(ctx context.Context, tokenHash string) (*domain.RefreshSession, error)
	// Rotate атомарно подменяет хэш и срок живой сессии, возвращая её новое состояние.
	Rotate(ctx context.Context, oldHash, newHash string, expiresAt time.Time) (*domain.RefreshSession, error)
	// Revoke удаляет сессию; отсутствие сессии ошибкой не считается.
	Revoke(ctx context.Context, tokenHash string) error
	// RevokeAllByUser удаляет все сессии пользователя.
	RevokeAllByUser(ctx context.Context, userID string) error
}
