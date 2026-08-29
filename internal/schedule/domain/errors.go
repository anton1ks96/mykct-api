package domain

import "errors"

var (
	// ErrPortalUnavailable - портал колледжа не ответил или ответил не расписанием.
	ErrPortalUnavailable = errors.New("college portal unavailable")
	// ErrScheduleUnavailable - портал недоступен, а снимка расписания в кэше нет.
	ErrScheduleUnavailable = errors.New("schedule unavailable")
	// ErrClassDetailsUnavailable - портал недоступен, а деталей занятия в кэше нет.
	ErrClassDetailsUnavailable = errors.New("class details unavailable")
)
