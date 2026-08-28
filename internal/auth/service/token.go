package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/anton1ks96/mykct-api/internal/auth/domain"
	"github.com/golang-jwt/jwt/v5"
)

// refreshTokenBytes - длина случайной части refresh-токена. 256 бит энтропии
// делают перебор невозможным, поэтому хэш в базе солить не нужно.
const refreshTokenBytes = 32

// tokenManager выпускает access-JWT и opaque refresh-токены.
type tokenManager struct {
	signingKey     []byte
	accessTokenTTL time.Duration
}

// newTokenManager создаёт менеджер токенов с ключом подписи и временем жизни access.
func newTokenManager(signingKey string, accessTokenTTL time.Duration) *tokenManager {
	return &tokenManager{
		signingKey:     []byte(signingKey),
		accessTokenTTL: accessTokenTTL,
	}
}

// newAccessToken выпускает HS256-JWT с профилем пользователя в claims.
// Профиль внутри токена позволяет проверять его без обращения к базе.
func (m *tokenManager) newAccessToken(user *domain.UserExtended) (string, error) {
	now := time.Now()

	claims := jwt.MapClaims{
		"user_id":  user.ID,
		"username": user.Username,
		"role":     user.Role,
		"iat":      now.Unix(),
		"exp":      now.Add(m.accessTokenTTL).Unix(),
	}

	// Необязательные поля пропускаем, чтобы не раздувать токен пустыми строками
	if user.AcademicGroup != "" {
		claims["academic_group"] = user.AcademicGroup
	}
	if user.Profile != "" {
		claims["profile"] = user.Profile
	}
	if user.Subgroup != "" {
		claims["subgroup"] = user.Subgroup
	}
	if user.EnglishGroup != "" {
		claims["english_group"] = user.EnglishGroup
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.signingKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign access token: %w", err)
	}

	return signed, nil
}

// parseAccessToken проверяет подпись и срок токена и восстанавливает профиль
// из claims. Срок и алгоритм подписи проверяет сам jwt.Parse.
func (m *tokenManager) parseAccessToken(token string) (*domain.UserExtended, error) {
	parsed, err := jwt.Parse(token, func(*jwt.Token) (any, error) {
		return m.signingKey, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidToken, err)
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("%w: unexpected claims format", domain.ErrInvalidToken)
	}

	userID, _ := claims["user_id"].(string)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id claim is missing", domain.ErrInvalidToken)
	}

	username, _ := claims["username"].(string)
	role, _ := claims["role"].(string)
	academicGroup, _ := claims["academic_group"].(string)
	profile, _ := claims["profile"].(string)
	subgroup, _ := claims["subgroup"].(string)
	englishGroup, _ := claims["english_group"].(string)

	return &domain.UserExtended{
		ID:            userID,
		Username:      username,
		Role:          role,
		AcademicGroup: academicGroup,
		Profile:       profile,
		Subgroup:      subgroup,
		EnglishGroup:  englishGroup,
	}, nil
}

// newRefreshToken возвращает случайный opaque-токен. В отличие от access это не
// JWT: он ничего не несёт внутри и проверяется только по базе.
func newRefreshToken() (string, error) {
	buf := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// hashToken считает SHA-256 в hex - именно он хранится в базе вместо токена.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
