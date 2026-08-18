package classify_test

import (
	"testing"

	"github.com/lacsar712/hookrelay/internal/classify"
)

func TestHTTPStatus(t *testing.T) {
	cases := []struct {
		code int
		kind classify.Kind
	}{
		{200, classify.Success},
		{204, classify.Success},
		{408, classify.Retryable},
		{429, classify.Retryable},
		{500, classify.Retryable},
		{503, classify.Retryable},
		{400, classify.Terminal},
		{404, classify.Terminal},
		{422, classify.Terminal},
	}
	for _, tc := range cases {
		if got := classify.HTTPStatus(tc.code); got != tc.kind {
			t.Fatalf("status %d: got %s want %s", tc.code, got, tc.kind)
		}
	}
}
