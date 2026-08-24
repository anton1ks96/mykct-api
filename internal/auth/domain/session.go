package domain

import "time"

// RefreshSession - выданный refresh-токен со снимком профиля пользователя.
// Сам токен не хранится: в базе лежит только его хэш.
type RefreshSession struct {
	TokenHash     string // SHA-256 от refresh-токена в hex
	UserID        string
	Username      string
	Role          string
	AcademicGroup string
	Profile       string
	Subgroup      string
	EnglishGroup  string
	ExpiresAt     time.Time
	CreatedAt     time.Time
}

// User восстанавливает профиль пользователя из снимка в сессии.
func (s *RefreshSession) User() *UserExtended {
	return &UserExtended{
		ID:            s.UserID,
		Username:      s.Username,
		Role:          s.Role,
		AcademicGroup: s.AcademicGroup,
		Profile:       s.Profile,
		Subgroup:      s.Subgroup,
		EnglishGroup:  s.EnglishGroup,
	}
}
