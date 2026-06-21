package screens

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tomascosta29/Ledger/internal/application/ports"
)

// Filter is a parsed filter DSL expression. The DSL is a whitespace-
// separated list of `field:value` clauses; all are AND-combined.
//
// Supported fields:
//   desc:<substring>        description LIKE %substring%
//   partner:<substring>     partner_name LIKE %substring%
//   iban:<iban>             partner_iban = iban
//   min:<amount>            amount_minor >= amount*100
//   max:<amount>            amount_minor <= amount*100
//   sign:- or sign:+        amount_minor < 0 / > 0
//   category:<name>         category = name
//   bucket:<name>           bucket = name (looked up)
//   id:<n>                  raw_transaction_id = n
//
// A bare <amount> with no field is treated as `min:<amount>`.
type Filter struct {
	DescriptionLike *string
	PartnerName     *string
	PartnerIBAN     *string
	AmountMin       *int64
	AmountMax       *int64
	AmountSign      *string
	Category        *string
	BucketName      *string
	RawID           *int64
}

// Parse parses a filter DSL string. Empty input is valid (no clauses).
func Parse(input string) (Filter, error) {
	var f Filter
	clauses := tokenize(input)
	for _, c := range clauses {
		if !strings.Contains(c, ":") {
			// Bare number → min:<n>
			major, err := strconv.ParseFloat(c, 64)
			if err != nil {
				return f, fmt.Errorf("invalid clause: %q", c)
			}
			minor := int64(major * 100)
			f.AmountMin = &minor
			continue
		}
		field, value, _ := strings.Cut(c, ":")
		if value == "" {
			return f, fmt.Errorf("empty value for field %q", field)
		}
		switch strings.ToLower(field) {
		case "desc", "description":
			f.DescriptionLike = &value
		case "partner", "name":
			f.PartnerName = &value
		case "iban":
			f.PartnerIBAN = &value
		case "min":
			minor, err := parseMajor(value)
			if err != nil {
				return f, fmt.Errorf("min: %w", err)
			}
			f.AmountMin = &minor
		case "max":
			minor, err := parseMajor(value)
			if err != nil {
				return f, fmt.Errorf("max: %w", err)
			}
			f.AmountMax = &minor
		case "sign":
			if value != "+" && value != "-" {
				return f, fmt.Errorf("sign must be + or -")
			}
			f.AmountSign = &value
		case "cat", "category":
			f.Category = &value
		case "bucket":
			f.BucketName = &value
		case "id":
			id, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return f, fmt.Errorf("id: %w", err)
			}
			f.RawID = &id
		default:
			return f, fmt.Errorf("unknown field: %q", field)
		}
	}
	return f, nil
}

// Apply converts the parsed filter into the overlay's filter struct.
// The BucketName is not resolved here; the caller must resolve it to a
// bucket ID first (the TUI does this when it has access to the repo).
func (f Filter) Apply() ports.OverlayFilters {
	out := ports.OverlayFilters{
		SourceKinds: []ports.SourceKind{
			ports.SourceRaw,
			ports.SourceSplitHeader,
			ports.SourceSplitChild,
			ports.SourceGroup,
		},
	}
	if f.DescriptionLike != nil {
		out.DescriptionLike = f.DescriptionLike
	}
	if f.PartnerName != nil {
		out.PartnerName = f.PartnerName
	}
	if f.PartnerIBAN != nil {
		out.PartnerIBAN = f.PartnerIBAN
	}
	if f.AmountMin != nil {
		out.AmountMinMinor = f.AmountMin
	}
	if f.AmountMax != nil {
		out.AmountMaxMinor = f.AmountMax
	}
	if f.AmountSign != nil {
		out.AmountSign = f.AmountSign
	}
	if f.Category != nil {
		out.Category = f.Category
	}
	if f.RawID != nil {
		out.RawTransactionID = f.RawID
	}
	return out
}

func tokenize(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	var buf strings.Builder
	inQuote := false
	flush := func() {
		if buf.Len() > 0 {
			out = append(out, buf.String())
			buf.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case (r == ' ' || r == '\t') && !inQuote:
			flush()
		default:
			buf.WriteRune(r)
		}
	}
	flush()
	return out
}

func parseMajor(s string) (int64, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return int64(f * 100), nil
}
