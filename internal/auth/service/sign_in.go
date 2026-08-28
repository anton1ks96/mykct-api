package service

import (
	"context"
	"time"

	"github.com/anton1ks96/mykct-api/internal/auth/domain"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// SignIn проверяет учётные данные в каталоге, заводит refresh-сессию и выдаёт
// пару токенов.
func (s *Service) SignIn(ctx context.Context, input SignInInput) (*SignInOutput, error) {
	op := logger.NewLogOp(ctx, log, "SignIn")
	op.Started().Str("user_id", input.UserID).Msg("sign in requested")

	if input.UserID == "" || input.Password == "" {
		return nil, domain.ErrInvalidCredentials
	}

	// 1. Профиль пользователя: заглушка в тестовом режиме либо каталог колледжа
	user, err := s.resolveUser(ctx, input)
	if err != nil {
		logFailure(op, err, "sign in failed")
		return nil, err
	}

	// 2. Выпуск пары токенов
	accessToken, err := s.tokens.newAccessToken(user)
	if err != nil {
		op.Failed(err).Str("user_id", user.ID).Msg("failed to issue access token")
		return nil, err
	}

	refreshToken, err := newRefreshToken()
	if err != nil {
		op.Failed(err).Str("user_id", user.ID).Msg("failed to issue refresh token")
		return nil, err
	}

	// 3. Сессия хранит снимок профиля, чтобы refresh и access не ходили в каталог
	now := time.Now()
	session := &domain.RefreshSession{
		TokenHash:     hashToken(refreshToken),
		UserID:        user.ID,
		Username:      user.Username,
		Role:          user.Role,
		AcademicGroup: user.AcademicGroup,
		Profile:       user.Profile,
		Subgroup:      user.Subgroup,
		EnglishGroup:  user.EnglishGroup,
		ExpiresAt:     now.Add(s.refreshTokenTTL),
		CreatedAt:     now,
	}

	if err := s.sessions.Save(ctx, session); err != nil {
		op.Failed(err).Str("user_id", user.ID).Msg("failed to save refresh session")
		return nil, err
	}

	op.Completed().Str("user_id", user.ID).Str("role", user.Role).Msg("signed in")

	return &SignInOutput{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		AccessExpiresIn:  s.accessTokenTTL,
		RefreshExpiresIn: s.refreshTokenTTL,
		User:             user,
	}, nil
}

// resolveUser возвращает профиль пользователя из каталога или заглушку.
func (s *Service) resolveUser(ctx context.Context, input SignInInput) (*domain.UserExtended, error) {
	if s.testMode {
		return testUser(input.UserID), nil
	}

	if err := s.directory.Authenticate(ctx, input.UserID, input.Password); err != nil {
		return nil, err
	}

	user, err := s.directory.GetByID(ctx, input.UserID, input.Password)
	if err != nil {
		return nil, err
	}

	// Учебных групп у преподавателей и администраторов нет
	if user.Role == domain.RoleTeacher || user.Role == domain.RoleAdmin {
		return domain.NewUserExtended(user, &domain.UserGroups{}), nil
	}

	groups, err := s.directory.GetUserGroups(ctx, input.UserID, input.Password)
	if err != nil {
		// Неполный профиль лучше отказа во входе
		log.Warn().Err(err).Str("user_id", input.UserID).Msg("failed to fetch user groups")
		groups = &domain.UserGroups{}
	}

	return domain.NewUserExtended(user, groups), nil
}

// testUser - заглушка профиля для разработки без доступа к каталогу колледжа.
func testUser(userID string) *domain.UserExtended {
	return &domain.UserExtended{
		ID:            userID,
		Username:      "Тестовый Пользователь",
		Role:          domain.RoleStudent,
		AcademicGroup: "ИТ25-11",
		Profile:       "BE",
		Subgroup:      "Подгр1",
		EnglishGroup:  "B1.21",
	}
}
