package csv

import (
	"strings"
	"testing"

	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
)

func TestComputeSourceHashDeterministic(t *testing.T) {
	in := HashInput{
		ProfileName:    "erste",
		ProfileVersion: 1,
		BookingDate:    "2026-04-30",
		AmountMinor:    -3090,
		Currency:       valueobjects.EUR,
		PartnerName:    "Partner_1",
		Description:    "Partner_1 2361 K1 30.04. 16:53",
	}
	a := ComputeSourceHash(in)
	b := ComputeSourceHash(in)
	if a != b {
		t.Fatalf("hash not deterministic: %s != %s", a, b)
	}
	if len(a) != 64 {
		t.Fatalf("expected 64 hex chars, got %d (%s)", len(a), a)
	}
}

func TestComputeSourceHashIgnoresCase(t *testing.T) {
	a := ComputeSourceHash(HashInput{
		ProfileName: "erste", ProfileVersion: 1,
		BookingDate: "2026-04-30", AmountMinor: -100, Currency: valueobjects.EUR,
		PartnerName: "ACME GMBH", Description: "Invoice 42",
	})
	b := ComputeSourceHash(HashInput{
		ProfileName: "erste", ProfileVersion: 1,
		BookingDate: "2026-04-30", AmountMinor: -100, Currency: valueobjects.EUR,
		PartnerName: "acme gmbh", Description: "INVOICE 42",
	})
	if a != b {
		t.Fatalf("hash should be case-insensitive on partner/desc: %s vs %s", a, b)
	}
}

func TestComputeSourceHashChangesWithAmount(t *testing.T) {
	a := ComputeSourceHash(HashInput{ProfileName: "erste", BookingDate: "2026-04-30", AmountMinor: -100, Currency: valueobjects.EUR, PartnerName: "ACME", Description: "X"})
	b := ComputeSourceHash(HashInput{ProfileName: "erste", BookingDate: "2026-04-30", AmountMinor: -101, Currency: valueobjects.EUR, PartnerName: "ACME", Description: "X"})
	if a == b {
		t.Fatal("hash should change with amount")
	}
}

func TestComputeSourceHashChangesWithProfile(t *testing.T) {
	base := HashInput{BookingDate: "2026-04-30", AmountMinor: -100, Currency: valueobjects.EUR, PartnerName: "ACME", Description: "X"}
	a := ComputeSourceHash(base)
	base.ProfileName = "erste"
	b := ComputeSourceHash(base)
	base.ProfileName = "revolut"
	c := ComputeSourceHash(base)
	if a == b || b == c || a == c {
		t.Fatalf("profile should change hash: a=%s b=%s c=%s", a, b, c)
	}
}

func TestNormalizePartnerName(t *testing.T) {
	cases := map[string]string{
		"":            "",
		"ACME gmbh":   "Acme Gmbh",
		"partner_one": "Partner_one",
		"  spaced  ":  "Spaced",
		"ALLCAPS":     "Allcaps",
	}
	for in, want := range cases {
		got := normalizePartnerName(in)
		if !strings.EqualFold(got, want) && got != want {
			t.Fatalf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}