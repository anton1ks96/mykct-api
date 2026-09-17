package domain

import "errors"

// ErrPortalUnavailable - портал колледжа не ответил или ответил не посещаемостью.
var ErrPortalUnavailable = errors.New("college portal unavailable")
