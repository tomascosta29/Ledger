package csv

import (
	"strings"
	"testing"

	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
)

func TestParseErste(t *testing.T) {
	profile := ErsteProfile()
	input := `"Own account name","Own IBAN","Booking Date","Partner Name","Partner IBAN","BIC/SWIFT","Partner Account Number","Bank code","Amount","Currency","Booking details","Booking Reference","Note","Highlights","Valuation Date","Virtual card number","Paid with","App","Payment Reference","Mandate ID","Creditor ID","Verification of Payee","This IBAN is registered to"
"Giro account","AT00XXXXXXXXXXXXXXXX","30.04.2026","Partner_1","","","XXXXX600","20111","-30.90","EUR","Partner_1 2361 K1 30.04. 16:53","REF-CE52B0B82C","","0","30.04.2026","XXXX-XXXX-XXXX-6596","|samsung|samsung|SM-S926B","Google Pay","","","","",""`

	rows, err := ParseFile(profile, strings.NewReader(input), ParseOptions{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.EffectiveDate != "2026-04-30" {
		t.Errorf("date = %q", r.EffectiveDate)
	}
	if r.AmountMinor != -3090 {
		t.Errorf("amount = %d, want -3090", r.AmountMinor)
	}
	if r.Currency != valueobjects.EUR {
		t.Errorf("currency = %q", r.Currency)
	}
	if r.PartnerName != "Partner_1" {
		t.Errorf("partner = %q", r.PartnerName)
	}
	if r.Description != "Partner_1 2361 K1 30.04. 16:53" {
		t.Errorf("desc = %q", r.Description)
	}
}

func TestParseErsteWithThousandsSep(t *testing.T) {
	profile := ErsteProfile()
	input := `"Own account name","Own IBAN","Booking Date","Partner Name","Partner IBAN","Amount","Currency","Booking details"
"X","Y","02.01.2026","P","","-10,000.00","EUR","George-Transfer"`

	rows, err := ParseFile(profile, strings.NewReader(input), ParseOptions{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].AmountMinor != -1000000 {
		t.Fatalf("amount = %d, want -1000000", rows[0].AmountMinor)
	}
}

func TestParseErsteWithBOM(t *testing.T) {
	profile := ErsteProfile()
	body := "\uFEFF\"Booking Date\",\"Amount\",\"Currency\",\"Booking details\",\"Partner Name\"\n30.04.2026,-30.90,EUR,Test,ACME"
	rows, err := ParseFile(profile, strings.NewReader(body), ParseOptions{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 || rows[0].AmountMinor != -3090 {
		t.Fatalf("unexpected: %+v", rows)
	}
}

func TestParseRevolut(t *testing.T) {
	profile := RevolutProfile()
	input := `Type,Product,Started Date,Completed Date,Description,Amount,Fee,Currency,State,Balance
Transfer,Savings,2026-03-27 09:21:45,2026-03-27 09:21:45,To pocket EUR Project Savings from EUR,1275.00,0.00,EUR,COMPLETED,1275.00
Card Payment,Current,2026-04-30 18:25:11,,YouTube,-7.49,0.00,EUR,PENDING,`

	rows, err := ParseFile(profile, strings.NewReader(input), ParseOptions{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("PENDING should be filtered; want 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.EffectiveDate != "2026-03-27" {
		t.Errorf("date = %q", r.EffectiveDate)
	}
	if r.AmountMinor != 127500 {
		t.Errorf("amount = %d, want 127500", r.AmountMinor)
	}
	if r.Description != "To pocket EUR Project Savings from EUR" {
		t.Errorf("desc = %q", r.Description)
	}
}

func TestParseMissingColumn(t *testing.T) {
	profile := ErsteProfile()
	input := "Wrong,Header\nfoo,bar"
	_, err := ParseFile(profile, strings.NewReader(input), ParseOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStripBOM(t *testing.T) {
	with := []byte{0xEF, 0xBB, 0xBF, 'a', 'b', 'c'}
	if got := stripBOM(with); string(got) != "abc" {
		t.Fatalf("got %q", got)
	}
	without := []byte("abc")
	if got := stripBOM(without); string(got) != "abc" {
		t.Fatalf("stripped non-BOM: %q", got)
	}
}

func TestParseAmountThousandsSep(t *testing.T) {
	cases := []struct {
		raw      string
		currency valueobjects.Currency
		decimal  string
		thousand string
		want     int64
		wantErr  bool
	}{
		{"-30.90", valueobjects.EUR, ".", ",", -3090, false},
		{"-10,000.00", valueobjects.EUR, ".", ",", -1000000, false},
		{"-1", valueobjects.EUR, ".", ",", -100, false},
		{"100", valueobjects.EUR, ".", ",", 10000, false},
		{"-1.234,56", valueobjects.EUR, ",", ".", -123456, false},
		{"", valueobjects.EUR, ".", ",", 0, true},
	}
	for _, tc := range cases {
		got, err := parseAmount(tc.raw, tc.currency, tc.decimal, tc.thousand)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseAmount(%q) err=%v wantErr=%v", tc.raw, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("parseAmount(%q) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}