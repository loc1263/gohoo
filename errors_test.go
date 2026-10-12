package gohoo

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestStatusErrorIs(t *testing.T) {
	cases := []struct {
		status int
		target error
		want   bool
	}{
		{http.StatusTooManyRequests, ErrRateLimit, true},
		{http.StatusNotFound, ErrNotFound, true},
		{http.StatusUnauthorized, ErrUnauthorized, true},
		{http.StatusInternalServerError, ErrRateLimit, false},
		{http.StatusNotFound, errors.New("otro"), false},
	}
	for _, tc := range cases {
		err := &StatusError{StatusCode: tc.status}
		if got := errors.Is(err, tc.target); got != tc.want {
			t.Errorf("errors.Is(HTTP %d, %v) = %v, want %v", tc.status, tc.target, got, tc.want)
		}
	}
}

func TestChartAPIErrorGeneric(t *testing.T) {
	err := chartAPIError("AAPL", "Bad Request", "Invalid input")
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrRateLimit) {
		t.Errorf("error genérico clasificado como sentinela: %v", err)
	}
	if !strings.Contains(err.Error(), "devolvió error: Bad Request: Invalid input") {
		t.Errorf("err = %q", err)
	}
}
