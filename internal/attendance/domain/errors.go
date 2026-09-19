package domain

import "errors"

// ErrPortalUnavailable - портал колледжа не ответил или ответил не посещаемостью.
var ErrPortalUnavailable = errors.New("college portal unavailable")

// ErrLeaderboardDisabled - рейтинг выключен в настройках сервиса.
var ErrLeaderboardDisabled = errors.New("leaderboard is disabled")

// ErrLeaderboardForbidden - рейтинг просит не студент либо студент без группы.
var ErrLeaderboardForbidden = errors.New("leaderboard is not available for this user")

// ErrLeaderboardTooSmall - на курсе слишком мало участников, чтобы рейтинг
// оставался анонимным.
var ErrLeaderboardTooSmall = errors.New("leaderboard cohort is too small")
