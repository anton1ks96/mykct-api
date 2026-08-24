package service

import (
	"time"

	"github.com/anton1ks96/mykct-api/internal/auth/domain"
)

// SignInInput - учётные данные для входа.
type SignInInput struct {
	UserID   string
	Password string
}

// SignInOutput - пара токенов и профиль вошедшего пользователя.
type SignInOutput struct {
	AccessToken      string
	RefreshToken     string
	AccessExpiresIn  time.Duration
	RefreshExpiresIn time.Duration
	User             *domain.UserExtended
}

// AccessTokenOutput - новый access-токен и профиль из снимка сессии.
type AccessTokenOutput struct {
	AccessToken string
	ExpiresIn   time.Duration
	User        *domain.UserExtended
}

// RefreshTokenOutput - новый refresh-токен взамен предъявленного.
type RefreshTokenOutput struct {
	RefreshToken string
	ExpiresIn    time.Duration
}
