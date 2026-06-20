package screens

import (
	"testing"
)

func TestParseEmpty(t *testing.T) {
	f, err := Parse("")
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if f.DescriptionLike != nil {
		t.Fatalf("expected no clauses")
	}
}

func TestParseSingleClause(t *testing.T) {
	f, err := Parse("desc:rent")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if f.DescriptionLike == nil || *f.DescriptionLike != "rent" {
		t.Fatalf("desc not parsed: %+v", f)
	}
}

func TestParseMultipleClauses(t *testing.T) {
	f, err := Parse(`sign:- min:100 max:500 partner:café category:want`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if f.AmountSign == nil || *f.AmountSign != "-" {
		t.Errorf("sign not parsed")
	}
	if f.AmountMin == nil || *f.AmountMin != 10000 {
		t.Errorf("min not parsed: %v", f.AmountMin)
	}
	if f.AmountMax == nil || *f.AmountMax != 50000 {
		t.Errorf("max not parsed: %v", f.AmountMax)
	}
	if f.PartnerName == nil || *f.PartnerName != "café" {
		t.Errorf("partner not parsed: %v", f.PartnerName)
	}
	if f.Category == nil || *f.Category != "want" {
		t.Errorf("category not parsed: %v", f.Category)
	}
}

func TestParseBareAmount(t *testing.T) {
	f, err := Parse("42")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if f.AmountMin == nil || *f.AmountMin != 4200 {
		t.Fatalf("bare amount: %v", f.AmountMin)
	}
}

func TestParseQuotedValue(t *testing.T) {
	f, err := Parse(`partner:"Acme Corp" desc:invoice`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if f.PartnerName == nil || *f.PartnerName != "Acme Corp" {
		t.Errorf("quoted partner: %v", f.PartnerName)
	}
	if f.DescriptionLike == nil || *f.DescriptionLike != "invoice" {
		t.Errorf("desc: %v", f.DescriptionLike)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []string{
		"unknown:field",
		"sign:0",
		"min:abc",
		"id:abc",
		"empty:",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if _, err := Parse(c); err == nil {
				t.Fatalf("expected error for %q", c)
			}
		})
	}
}
