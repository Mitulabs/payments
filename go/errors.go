package mitu

import (
	"errors"
	"net/http"
)

// ErrConfig reports options or a Price that cannot be used.
var ErrConfig = errors.New("mitu: invalid configuration")

var (
	errInvalidCurrency = errors.New("must be a currency code, such as \"USD\" or \"NGN\"")
	errInvalidAmount   = errors.New("must be a decimal greater than 0 with at most 2 decimal places, such as \"5000\" or \"12.50\"")
)

// apiError is a failed call to Mitu. status is 0 when no response arrived.
// Its message never contains the key, a credential or a response body.
type apiError struct {
	status  int
	message string
	err     error
}

func (e *apiError) Error() string { return "mitu: " + e.message }

func (e *apiError) Unwrap() error { return e.err }

// unavailable reports whether Mitu could not answer: no response, rate
// limiting or a server error. The route then answers 503.
func (e *apiError) unavailable() bool {
	return e.status == 0 || e.status == http.StatusTooManyRequests || e.status >= http.StatusInternalServerError
}
