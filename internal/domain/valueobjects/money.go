package valueobjects

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

type Currency string

const (
	EUR Currency = "EUR"
	USD Currency = "USD"
	GBP Currency = "GBP"
	CHF Currency = "CHF"
)

func (c Currency) Valid() bool {
	switch c {
	case EUR, USD, GBP, CHF:
		return true
	}
	return false
}

func (c Currency) MinorUnits() int32 {
	switch c {
	case EUR, USD, GBP, CHF:
		return 2
	}
	return 0
}

type Money struct {
	Amount   int64
	Currency Currency
}

var (
	ErrCurrencyMismatch = errors.New("currency mismatch")
	ErrUnknownCurrency  = errors.New("unknown currency")
)

func New(amountMinor int64, c Currency) (Money, error) {
	if !c.Valid() {
		return Money{}, fmt.Errorf("%w: %q", ErrUnknownCurrency, c)
	}
	return Money{Amount: amountMinor, Currency: c}, nil
}

func MustNew(amountMinor int64, c Currency) Money {
	m, err := New(amountMinor, c)
	if err != nil {
		panic(err)
	}
	return m
}

func ParseDecimal(s string, c Currency) (Money, error) {
	if !c.Valid() {
		return Money{}, fmt.Errorf("%w: %q", ErrUnknownCurrency, c)
	}
	cleaned := strings.TrimSpace(s)
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "\u00a0", "")

	var sign int64 = 1
	switch {
	case strings.HasPrefix(cleaned, "-"):
		sign = -1
		cleaned = cleaned[1:]
	case strings.HasPrefix(cleaned, "+"):
		cleaned = cleaned[1:]
	}
	if cleaned == "" {
		return Money{}, fmt.Errorf("invalid amount: %q", s)
	}

	cleaned = normaliseDecimal(cleaned)
	if cleaned == "" {
		return Money{}, fmt.Errorf("invalid amount: %q", s)
	}

	rat, ok := new(big.Rat).SetString(cleaned)
	if !ok {
		return Money{}, fmt.Errorf("invalid amount: %q", s)
	}

	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(c.MinorUnits())), nil)
	scaled := new(big.Rat).Mul(rat, new(big.Rat).SetInt(scale))
	minor := new(big.Int).Quo(scaled.Num(), scaled.Denom())
	if !scaled.IsInt() {
		return Money{}, fmt.Errorf("amount %q exceeds minor-unit precision for %s", s, c)
	}
	if !minor.IsInt64() {
		return Money{}, fmt.Errorf("amount %q out of int64 range", s)
	}

	return Money{Amount: sign * minor.Int64(), Currency: c}, nil
}

func normaliseDecimal(s string) string {
	lastDot := strings.LastIndex(s, ".")
	lastComma := strings.LastIndex(s, ",")

	switch {
	case lastDot == -1 && lastComma == -1:
		return s
	case lastDot != -1 && lastComma != -1:
		if lastDot > lastComma {
			return strings.ReplaceAll(strings.ReplaceAll(s, ",", ""), ".", ".")
		}
		return strings.ReplaceAll(strings.ReplaceAll(s, ".", ""), ",", ".")
	case lastDot != -1:
		if strings.Count(s, ".") > 1 {
			return strings.ReplaceAll(s, ".", "")
		}
		return s
	case lastComma != -1:
		if strings.Count(s, ",") > 1 {
			return strings.ReplaceAll(s, ",", "")
		}
		return strings.Replace(s, ",", ".", 1)
	}
	return s
}

func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.Currency, other.Currency)
	}
	return Money{Amount: m.Amount + other.Amount, Currency: m.Currency}, nil
}

func (m Money) Sub(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.Currency, other.Currency)
	}
	return Money{Amount: m.Amount - other.Amount, Currency: m.Currency}, nil
}

func (m Money) IsZero() bool     { return m.Amount == 0 }
func (m Money) IsPositive() bool { return m.Amount > 0 }
func (m Money) IsNegative() bool { return m.Amount < 0 }

func (m Money) Sign() int {
	switch {
	case m.Amount > 0:
		return 1
	case m.Amount < 0:
		return -1
	}
	return 0
}

func (m Money) DecimalString() string {
	if !m.Currency.Valid() {
		return fmt.Sprintf("%d ?", m.Amount)
	}
	minor := m.Currency.MinorUnits()
	scale := int64(1)
	for i := int32(0); i < minor; i++ {
		scale *= 10
	}
	abs := m.Amount
	if abs < 0 {
		abs = -abs
	}
	whole := abs / scale
	frac := abs % scale
	sign := ""
	if m.Amount < 0 {
		sign = "-"
	}
	if minor == 0 {
		return fmt.Sprintf("%s%d", sign, whole)
	}
	return fmt.Sprintf("%s%d.%0*d", sign, whole, minor, frac)
}

func (m Money) String() string {
	return fmt.Sprintf("%s %s", m.DecimalString(), m.Currency)
}

func (m Money) Equals(other Money) bool {
	return m.Amount == other.Amount && m.Currency == other.Currency
}
