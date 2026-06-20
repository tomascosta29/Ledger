package csv

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
)

type ParsedRow struct {
	EffectiveDate  string
	AmountMinor    int64
	Currency       valueobjects.Currency
	Description    string
	PartnerName    string
	PartnerIBAN    string
	RawDescription string
}

type ParseError struct {
	LineNumber int
	Field      string
	Reason     string
}

func (e *ParseError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("line %d: field %q: %s", e.LineNumber, e.Field, e.Reason)
	}
	return fmt.Sprintf("line %d: %s", e.LineNumber, e.Reason)
}

type ParseOptions struct {
	SkipHeaderRows int
}

func ParseFile(profile *Profile, r io.Reader, opts ParseOptions) ([]ParsedRow, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	data = stripBOM(data)

	reader := csv.NewReader(bytes.NewReader(data))
	if profile.Delimiter != "" {
		reader.Comma = rune(profile.Delimiter[0])
	}
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv read: %w", err)
	}
	if len(rows) == 0 {
		return nil, errors.New("empty file")
	}
	if opts.SkipHeaderRows > len(rows) {
		return nil, fmt.Errorf("skipHeaderRows %d > rows %d", opts.SkipHeaderRows, len(rows))
	}

	header := rows[0]
	idx, err := buildColumnIndex(profile, header)
	if err != nil {
		return nil, err
	}

	out := make([]ParsedRow, 0, len(rows)-1-opts.SkipHeaderRows)
	for i, row := range rows[1:] {
		if opts.SkipHeaderRows > 0 && i < opts.SkipHeaderRows {
			continue
		}
		if isEmptyRow(row) {
			continue
		}
		if profile.HasStateFilter() {
			state := ""
			if idx.state >= 0 && idx.state < len(row) {
				state = row[idx.state]
			}
			if state != profile.StateFilter {
				continue
			}
		}

		parsed, err := parseRow(profile, row, idx)
		if err != nil {
			return nil, err
		}
		parsed = shiftDate(parsed, profile.EffectiveDateShift)
		out = append(out, parsed)
	}
	return out, nil
}

type colIndex struct {
	bookingDate    int
	amount         int
	description    int
	currency       int
	partnerName    int
	partnerIBAN    int
	state          int
	rawDescription int
}

func buildColumnIndex(p *Profile, header []string) (*colIndex, error) {
	idx := &colIndex{
		bookingDate: -1, amount: -1, description: -1,
		currency: -1, partnerName: -1, partnerIBAN: -1,
		state: -1, rawDescription: -1,
	}
	missing := []string{}
	for i, name := range header {
		switch name {
		case p.Columns.BookingDate:
			idx.bookingDate = i
		case p.Columns.Amount:
			idx.amount = i
		case p.Columns.Description:
			idx.description = i
		case p.Columns.Currency:
			idx.currency = i
		case p.Columns.PartnerName:
			idx.partnerName = i
		case p.Columns.PartnerIBAN:
			idx.partnerIBAN = i
		case p.Columns.State:
			idx.state = i
		case p.Columns.RawDescription:
			idx.rawDescription = i
		}
	}
	if idx.bookingDate == -1 {
		missing = append(missing, p.Columns.BookingDate)
	}
	if idx.amount == -1 {
		missing = append(missing, p.Columns.Amount)
	}
	if idx.description == -1 {
		missing = append(missing, p.Columns.Description)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required columns in header: %s", strings.Join(missing, ", "))
	}
	return idx, nil
}

func parseRow(p *Profile, row []string, idx *colIndex) (ParsedRow, error) {
	effDate, err := parseDateField(row[idx.bookingDate], p.DateFormat)
	if err != nil {
		return ParsedRow{}, &ParseError{LineNumber: -1, Field: p.Columns.BookingDate, Reason: err.Error()}
	}

	currencyStr := p.DefaultCurrency
	if idx.currency >= 0 && idx.currency < len(row) {
		if c := strings.TrimSpace(row[idx.currency]); c != "" {
			currencyStr = c
		}
	}
	currency := valueobjects.Currency(strings.ToUpper(currencyStr))
	if !currency.Valid() {
		return ParsedRow{}, &ParseError{Field: p.Columns.Currency, Reason: fmt.Sprintf("unknown currency %q", currencyStr)}
	}

	amount, err := parseAmount(row[idx.amount], currency, p.DecimalSep, p.ThousandsSep)
	if err != nil {
		return ParsedRow{}, &ParseError{Field: p.Columns.Amount, Reason: err.Error()}
	}

	desc := safeField(row, idx.description)
	rawDesc := desc
	if idx.rawDescription >= 0 {
		rawDesc = safeField(row, idx.rawDescription)
	}

	partnerName := safeField(row, idx.partnerName)
	partnerIBAN := safeField(row, idx.partnerIBAN)

	return ParsedRow{
		EffectiveDate:  effDate,
		AmountMinor:    amount,
		Currency:       currency,
		Description:    desc,
		PartnerName:    partnerName,
		PartnerIBAN:    partnerIBAN,
		RawDescription: rawDesc,
	}, nil
}

func parseDateField(raw string, format string) (string, error) {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return "", errors.New("empty date")
	}
	t, err := time.Parse(format, clean)
	if err != nil {
		return "", fmt.Errorf("parse %q with %q: %w", clean, format, err)
	}
	return t.Format("2006-01-02"), nil
}

func parseAmount(raw string, currency valueobjects.Currency, decimalSep, thousandsSep string) (int64, error) {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return 0, errors.New("empty amount")
	}
	if thousandsSep != "" && thousandsSep != decimalSep {
		cleaned = strings.ReplaceAll(cleaned, thousandsSep, "")
	}
	if decimalSep != "." {
		cleaned = strings.ReplaceAll(cleaned, decimalSep, ".")
	}
	money, err := valueobjects.ParseDecimal(cleaned, currency)
	if err != nil {
		return 0, err
	}
	return money.Amount, nil
}

func shiftDate(r ParsedRow, days int) ParsedRow {
	if days == 0 {
		return r
	}
	t, err := time.Parse("2006-01-02", r.EffectiveDate)
	if err != nil {
		return r
	}
	t = t.AddDate(0, 0, days)
	r.EffectiveDate = t.Format("2006-01-02")
	return r
}

func stripBOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}
	return data
}

func isEmptyRow(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func safeField(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func IsBOMOnly(s string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return s != ""
}
