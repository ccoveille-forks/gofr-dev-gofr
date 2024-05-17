package gofrerror

import (
	"net/http"
	"strings"

	"github.com/pkg/errors"
)

// errorGoFr represents a generic GoFr error.
type errorGoFr struct {
	error
	message string
}

// Error returns the formatted error message.
func (e *errorGoFr) Error() string {
	if e.error != nil {
		return e.error.Error()
	}

	return e.message
}

//nolint:revive // New creates a new GoFr error and wraps the error with the provided message.
func New(err error, message ...string) *errorGoFr {
	errMsg := strings.Join(message, " ")

	if errMsg != "" {
		return &errorGoFr{
			error:   errors.Wrap(err, errMsg),
			message: errMsg,
		}
	}

	return &errorGoFr{
		error: err,
	}
}

func (e *errorGoFr) StatusCode() int {
	return http.StatusInternalServerError
}
