package domain

import (
	"errors"
	"fmt"
)

// Sentinel errors used across layers so callers (and the HTTP layer) can map
// them to the right status codes without string matching.
var (
	ErrNotFound        = errors.New("dispute not found")
	ErrInvalidInput    = errors.New("invalid input")
	ErrNotAnalyzed     = errors.New("dispute has not been analysed yet")
	ErrAlreadySettled  = errors.New("dispute is already settled")
	ErrNotNegotiating  = errors.New("dispute is not in a negotiable state")
)

// WrapInvalid tags an arbitrary error (typically a request-decoding failure) as
// invalid input so the transport layer maps it to a 400.
func WrapInvalid(err error) error {
	return fmt.Errorf("%w: %v", ErrInvalidInput, err)
}

// FormatINR renders whole rupees as an Indian-grouped currency string, e.g.
// 250000 -> "₹2,50,000". This grouping (lakhs/crores) is what Indian users
// expect and what appears on settlement documents.
func FormatINR(rupees int64) string {
	neg := rupees < 0
	if neg {
		rupees = -rupees
	}
	s := fmt.Sprintf("%d", rupees)
	n := len(s)
	if n <= 3 {
		if neg {
			return "₹-" + s
		}
		return "₹" + s
	}
	// last three digits, then groups of two
	out := s[n-3:]
	s = s[:n-3]
	for len(s) > 2 {
		out = s[len(s)-2:] + "," + out
		s = s[:len(s)-2]
	}
	out = s + "," + out
	if neg {
		return "₹-" + out
	}
	return "₹" + out
}
