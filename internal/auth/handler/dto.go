package handler

import "github.com/anton1ks96/mykct-api/internal/auth/domain"

// signInRequest - учётные данные для входа.
type signInRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required,min=6"`
}

// refreshTokenRequest - предъявляемый refresh-токен. Общий для выхода,
// ротации и выдачи access-токена.
type refreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// userInfo - профиль пользователя в ответах.
type userInfo struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Role          string `json:"role"`
	AcademicGroup string `json:"academic_group,omitempty"`
	Profile       string `json:"profile,omitempty"`
	Subgroup      string `json:"subgroup,omitempty"`
	EnglishGroup  string `json:"english_group,omitempty"`
}

// signInResponse - пара токенов и профиль вошедшего пользователя.
type signInResponse struct {
	AccessToken      string   `json:"access_token"`
	RefreshToken     string   `json:"refresh_token"`
	AccessExpiresIn  int      `json:"access_expires_in"`  // Секунд до истечения access
	RefreshExpiresIn int      `json:"refresh_expires_in"` // Секунд до истечения refresh
	User             userInfo `json:"user"`
}

// refreshTokenResponse - новый refresh-токен взамен предъявленного.
type refreshTokenResponse struct {
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// accessTokenResponse - новый access-токен и профиль из снимка сессии.
type accessTokenResponse struct {
	AccessToken string   `json:"access_token"`
	ExpiresIn   int      `json:"expires_in"`
	User        userInfo `json:"user"`
}

// signOutResponse - подтверждение выхода.
type signOutResponse struct {
	Message string `json:"message"`
}

// newUserInfo переводит доменный профиль в DTO ответа.
func newUserInfo(user *domain.UserExtended) userInfo {
	return userInfo{
		ID:            user.ID,
		Username:      user.Username,
		Role:          user.Role,
		AcademicGroup: user.AcademicGroup,
		Profile:       user.Profile,
		Subgroup:      user.Subgroup,
		EnglishGroup:  user.EnglishGroup,
	}
}
