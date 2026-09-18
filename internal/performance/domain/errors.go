package domain

import "errors"

// ErrPortalUnavailable - портал колледжа не ответил или ответил не успеваемостью.
var ErrPortalUnavailable = errors.New("college portal unavailable")
