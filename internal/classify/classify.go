package classify

import (
	"context"
	"errors"
	"net"
	"os"
	"syscall"
)

type Kind int

const (
	Success Kind = iota
	Retryable
	Terminal
)

func (k Kind) String() string {
	switch k {
	case Success:
		return "success"
	case Retryable:
		return "retryable"
	case Terminal:
		return "terminal"
	default:
		return "unknown"
	}
}

func HTTPStatus(code int) Kind {
	if code >= 200 && code <= 299 {
		return Success
	}
	switch code {
	case 408, 429:
		return Retryable
	}
	if code >= 500 && code <= 599 {
		return Retryable
	}
	if code >= 400 && code <= 499 {
		return Terminal
	}
	if code == 0 {
		return Retryable
	}
	return Terminal
}

func NetError(err error) Kind {
	if err == nil {
		return Success
	}
	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, os.ErrDeadlineExceeded) ||
		errors.Is(err, context.DeadlineExceeded) {
		return Retryable
	}
	// Anything satisfying net.Error whose Timeout() reports true is a
	// transient deadline (http.Client timeout, custom timeout errors
	// wrapped along the outbound chain). Inspect the method rather than a
	// sentinel so wrapped timeouts survive fmt.Errorf("...: %w", ...).
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return Retryable
	}
	return Terminal
}

func Combine(status int, err error) Kind {
	if err != nil {
		return NetError(err)
	}
	return HTTPStatus(status)
}
