package valueobjects

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrIBANEmpty    = errors.New("iban is empty")
	ErrIBANLength   = errors.New("iban has wrong length for country")
	ErrIBANFormat   = errors.New("iban format invalid")
	ErrIBANChecksum = errors.New("iban checksum invalid")
)

var (
	ibanAllowed        = regexp.MustCompile(`^[A-Z0-9]+$`)
	ibanCountryLengths = map[string]int{
		"AT": 20,
		"DE": 22,
		"CH": 21,
		"FR": 27,
		"GB": 22,
		"IT": 27,
		"ES": 24,
		"NL": 18,
		"BE": 16,
		"LU": 20,
		"PT": 25,
		"PL": 28,
		"CZ": 24,
		"SK": 24,
		"HU": 28,
		"RO": 24,
		"BG": 22,
		"HR": 21,
		"SI": 19,
		"FI": 18,
		"SE": 24,
		"DK": 18,
		"NO": 15,
		"IE": 22,
		"LI": 21,
		"MT": 31,
		"CY": 28,
		"EE": 20,
		"LV": 21,
		"LT": 20,
		"US": 0,
	}
)

type IBAN string

func (i IBAN) Country() string {
	s := string(i)
	if len(s) < 2 {
		return ""
	}
	return s[:2]
}

func (i IBAN) Valid() error {
	s := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(string(i)), " ", ""))
	if s == "" {
		return ErrIBANEmpty
	}
	if !ibanAllowed.MatchString(s) {
		return fmt.Errorf("%w: %q", ErrIBANFormat, s)
	}
	expected, ok := ibanCountryLengths[s[:2]]
	if !ok {
		return fmt.Errorf("%w: unknown country %q", ErrIBANFormat, s[:2])
	}
	if len(s) != expected {
		return fmt.Errorf("%w: %s expected %d, got %d", ErrIBANLength, s[:2], expected, len(s))
	}
	if !ibanChecksumValid(s) {
		return fmt.Errorf("%w: %s", ErrIBANChecksum, s)
	}
	return nil
}

func (i IBAN) Normalized() IBAN {
	return IBAN(strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(string(i)), " ", "")))
}

func ibanChecksumValid(s string) bool {
	rearranged := s[4:] + s[:4]
	var digits strings.Builder
	for _, r := range rearranged {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		} else {
			digits.WriteString(fmt.Sprintf("%d", r-'A'+10))
		}
	}
	return mod97(digits.String()) == 1
}

func mod97(s string) int {
	mod := 0
	for i := 0; i < len(s); i++ {
		mod = (mod*10 + int(s[i]-'0')) % 97
	}
	return mod
}
