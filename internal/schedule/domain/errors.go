package domain

import "errors"

var (
	// ErrPortalUnavailable - портал колледжа не ответил или ответил не расписанием.
	ErrPortalUnavailable = errors.New("college portal unavailable")
	// ErrScheduleUnavailable - портал недоступен, а снимка расписания в кэше нет.
	ErrScheduleUnavailable = errors.New("schedule unavailable")
	// ErrClassDetailsUnavailable - портал недоступен, а деталей занятия в кэше нет.
	ErrClassDetailsUnavailable = errors.New("class details unavailable")
	// ErrWeekStateNotFound - за неделей группы ещё не следили.
	ErrWeekStateNotFound = errors.New("week state not found")
	// ErrWeekStateExists - состояние недели уже завёл другой инстанс.
	ErrWeekStateExists = errors.New("week state already exists")
)
