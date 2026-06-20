package valueobjects

import (
	"errors"
	"testing"
)

func TestIBANValid(t *testing.T) {
	cases := []struct {
		name    string
		iban    string
		wantErr bool
	}{
		{"valid Austrian", "AT611904300234573201", false},
		{"valid German", "DE89370400440532013000", false},
		{"valid Swiss", "CH9300762011623852957", false},
		{"with spaces", "AT61 1904 3002 3457 3201", false},
		{"lowercase", "at611904300234573201", false},
		{"empty", "", true},
		{"unknown country", "XX1234567890123456789012", true},
		{"wrong length AT", "AT1234", true},
		{"bad checksum", "AT611904300234573202", true},
		{"special chars", "AT61-1904-3002", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			i := IBAN(tc.iban)
			err := i.Valid()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestIBANNormalized(t *testing.T) {
	i := IBAN("  at61 1904 3002 3457 3201 ")
	got := i.Normalized()
	if string(got) != "AT611904300234573201" {
		t.Fatalf("normalized = %q", got)
	}
}

func TestIBANChecksumKnownValues(t *testing.T) {
	known := []struct {
		iban string
		ok   bool
	}{
		{"AT611904300234573201", true},
		{"DE89370400440532013000", true},
		{"GB82WEST12345698765432", true},
		{"FR1420041010050500013M02606", true},
		{"ES9121000418450200051332", true},
		{"IT60X0542811101000000123456", true},
		{"NL91ABNA0417164300", true},
		{"BE68539007547034", true},
	}
	for _, k := range known {
		t.Run(k.iban, func(t *testing.T) {
			err := IBAN(k.iban).Valid()
			if k.ok && err != nil {
				t.Fatalf("expected valid, got %v", err)
			}
			if !k.ok && err == nil {
				t.Fatal("expected invalid")
			}
			if !errors.Is(err, ErrIBANChecksum) && !k.ok {
				_ = err
			}
		})
	}
}
