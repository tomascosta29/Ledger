package csv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltInProfiles(t *testing.T) {
	for _, name := range []string{"erste", "revolut"} {
		t.Run(name, func(t *testing.T) {
			p, err := GetProfile(name)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if p.Name != name {
				t.Fatalf("name = %q, want %q", p.Name, name)
			}
			if err := p.Validate(); err != nil {
				t.Fatalf("validate: %v", err)
			}
			if p.Columns.BookingDate == "" || p.Columns.Amount == "" {
				t.Fatalf("missing required column mapping: %+v", p.Columns)
			}
		})
	}
}

func TestUnknownProfile(t *testing.T) {
	_, err := GetProfile("does-not-exist")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCustomProfileFromTOML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEDGER_PROFILE_DIR", dir)

	toml := `
name = "mybank"
version = 1
delimiter = ","
quote_char = "\""
encoding = "utf-8"
date_format = "02.01.2006"
decimal_sep = ","
thousands_sep = "."
default_currency = "EUR"

[columns]
booking_date = "Buchungsdatum"
description = "Verwendungszweck"
amount = "Betrag"
currency = "Waehrung"
partner_name = "Name"
`
	if err := os.WriteFile(filepath.Join(dir, "mybank.toml"), []byte(toml), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	p, err := GetProfile("mybank")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if p.Name != "mybank" {
		t.Fatalf("name = %q", p.Name)
	}
	if p.Columns.BookingDate != "Buchungsdatum" {
		t.Fatalf("booking_date = %q", p.Columns.BookingDate)
	}
	if p.DecimalSep != "," {
		t.Fatalf("decimal_sep = %q", p.DecimalSep)
	}
}

func TestProfileValidation(t *testing.T) {
	p := &Profile{Name: "broken"}
	if err := p.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}