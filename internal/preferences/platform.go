package preferences

import (
	"errors"
	"runtime"
)

// SupportedPlatform is false on native Windows, where generation publication
// (an atomic rename over the `current` link) and transcript handling are not
// implemented yet. Reviews still work there; learning and the Yard do not.
var SupportedPlatform = runtime.GOOS != "windows"

// ErrUnsupportedPlatform is returned when the preference store is opened on a
// platform it does not support.
var ErrUnsupportedPlatform = errors.New("learned preferences are not supported on native Windows yet; use WSL")
