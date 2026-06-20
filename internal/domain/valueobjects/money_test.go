package valueobjects

import (
	"errors"
	"testing"
)

func TestNew(t *testing.T) {
	cases := []struct {
		name    string
		minor   int64
		curr    Currency
		want    Money
		wantErr bool
	}{
		{"EUR positive", 12345, EUR, Money{12345, EUR}, false},
		{"USD negative", -999, USD, Money{-999, USD}, false},
		{"unknown currency", 100, "XYZ", Money{}, true},
		{"empty currency", 100, "", Money{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := New(tc.minor, tc.curr)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseDecimal(t *testing.T) {
	cases := []struct {
		name    string
		s       string
		curr    Currency
		want    Money
		wantErr bool
	}{
		{"plain positive", "12.34", EUR, Money{1234, EUR}, false},
		{"plain negative", "-12.34", EUR, Money{-1234, EUR}, false},
		{"explicit plus", "+12.34", EUR, Money{1234, EUR}, false},
		{"comma decimal", "12,34", EUR, Money{1234, EUR}, false},
		{"with thousands sep", "1,234.56", EUR, Money{123456, EUR}, false},
		{"european thousands + decimal", "1.234,56", EUR, Money{123456, EUR}, false},
		{"thousands sep only no decimal", "1234", EUR, Money{123400, EUR}, false},
		{"trims spaces", "  12.34  ", EUR, Money{1234, EUR}, false},
		{"nbsp as thousand sep", "1\u00a0234,56", EUR, Money{123456, EUR}, false},
		{"empty", "", EUR, Money{}, true},
		{"just dot", ".", EUR, Money{}, true},
		{"garbage", "abc", EUR, Money{}, true},
		{"unknown currency", "12.34", "XYZ", Money{}, true},
		{"zero", "0", EUR, Money{0, EUR}, false},
		{"zero decimal", "0.00", EUR, Money{0, EUR}, false},
		{"USD no fractional unit in source", "100.5", USD, Money{10050, USD}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseDecimal(tc.s, tc.curr)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestAddSub(t *testing.T) {
	a := MustNew(1000, EUR)
	b := MustNew(250, EUR)
	c := MustNew(-150, EUR)
	d := MustNew(100, USD)

	got, err := a.Add(b)
	if err != nil || got.Amount != 1250 {
		t.Fatalf("Add: got %+v err %v, want 1250 EUR", got, err)
	}

	got, err = a.Sub(b)
	if err != nil || got.Amount != 750 {
		t.Fatalf("Sub: got %+v err %v, want 750 EUR", got, err)
	}

	got, err = a.Add(c)
	if err != nil || got.Amount != 850 {
		t.Fatalf("Add(neg): got %+v err %v, want 850 EUR", got, err)
	}

	if _, err := a.Add(d); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Add(cross-curr): err = %v, want ErrCurrencyMismatch", err)
	}
}

func TestSignAndEquality(t *testing.T) {
	pos := MustNew(100, EUR)
	neg := MustNew(-100, EUR)
	zero := MustNew(0, EUR)
	usd := MustNew(100, USD)

	if !pos.IsPositive() || pos.IsNegative() || pos.IsZero() {
		t.Fatalf("pos sign checks wrong")
	}
	if !neg.IsNegative() || neg.IsPositive() || neg.IsZero() {
		t.Fatalf("neg sign checks wrong")
	}
	if !zero.IsZero() || zero.IsPositive() || zero.IsNegative() {
		t.Fatalf("zero sign checks wrong")
	}

	if pos.Sign() != 1 || neg.Sign() != -1 || zero.Sign() != 0 {
		t.Fatalf("Sign() wrong")
	}

	if !pos.Equals(MustNew(100, EUR)) {
		t.Fatalf("Equals wrong on same")
	}
	if pos.Equals(usd) {
		t.Fatalf("Equals wrong: 100 EUR should not equal 100 USD")
	}
}

func TestDecimalString(t *testing.T) {
	cases := []struct {
		m    Money
		want string
	}{
		{MustNew(0, EUR), "0.00"},
		{MustNew(1, EUR), "0.01"},
		{MustNew(100, EUR), "1.00"},
		{MustNew(1234, EUR), "12.34"},
		{MustNew(-1234, EUR), "-12.34"},
		{MustNew(100, USD), "1.00"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.m.DecimalString(); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
