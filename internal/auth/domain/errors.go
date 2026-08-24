package domain

import "errors"

var (
	// ErrInvalidCredentials - неверный логин или пароль. Отсутствие пользователя в
	// каталоге намеренно неотличимо от неверного пароля.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrSessionNotFound - refresh-сессия неизвестна, истекла или уже использована.
	ErrSessionNotFound = errors.New("refresh session not found")
	// ErrInvalidToken - access-токен повреждён, просрочен или подписан чужим ключом.
	ErrInvalidToken = errors.New("invalid access token")
	// ErrDirectoryUnavailable - каталог LDAP недоступен или ответил ошибкой.
	ErrDirectoryUnavailable = errors.New("directory unavailable")
	// ErrRoleNotDetermined - роль не выводится из групп и расположения записи.
	ErrRoleNotDetermined = errors.New("user role not determined")
)
