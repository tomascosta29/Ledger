package csv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type ColumnMapping struct {
	BookingDate    string `toml:"booking_date"`
	Description    string `toml:"description"`
	Amount         string `toml:"amount"`
	Currency       string `toml:"currency"`
	PartnerName    string `toml:"partner_name"`
	PartnerIBAN    string `toml:"partner_iban"`
	Fee            string `toml:"fee"`
	State          string `toml:"state"`
	RawDescription string `toml:"raw_description"`
}

type Profile struct {
	Name               string        `toml:"name"`
	Version            int           `toml:"version"`
	Delimiter          string        `toml:"delimiter"`
	QuoteChar          string        `toml:"quote_char"`
	Encoding           string        `toml:"encoding"`
	DateFormat         string        `toml:"date_format"`
	DecimalSep         string        `toml:"decimal_sep"`
	ThousandsSep       string        `toml:"thousands_sep"`
	StateFilter        string        `toml:"state_filter"`
	DefaultCurrency    string        `toml:"default_currency"`
	EffectiveDateShift int           `toml:"effective_date_shift"`
	Columns            ColumnMapping `toml:"columns"`
}

func (p *Profile) HasStateFilter() bool {
	return p.Columns.State != "" && p.StateFilter != ""
}

func ErsteProfile() *Profile {
	return &Profile{
		Name:               "erste",
		Version:            1,
		Delimiter:          ",",
		QuoteChar:          "\"",
		Encoding:           "utf-8",
		DateFormat:         "02.01.2006",
		DecimalSep:         ".",
		ThousandsSep:       ",",
		StateFilter:        "",
		DefaultCurrency:    "EUR",
		EffectiveDateShift: 0,
		Columns: ColumnMapping{
			BookingDate: "Booking Date",
			Description: "Booking details",
			Amount:      "Amount",
			Currency:    "Currency",
			PartnerName: "Partner Name",
			PartnerIBAN: "Partner IBAN",
		},
	}
}

func RevolutProfile() *Profile {
	return &Profile{
		Name:               "revolut",
		Version:            1,
		Delimiter:          ",",
		QuoteChar:          "\"",
		Encoding:           "utf-8",
		DateFormat:         "2006-01-02 15:04:05",
		DecimalSep:         ".",
		ThousandsSep:       ",",
		StateFilter:        "COMPLETED",
		DefaultCurrency:    "EUR",
		EffectiveDateShift: 0,
		Columns: ColumnMapping{
			BookingDate: "Completed Date",
			Description: "Description",
			Amount:      "Amount",
			Currency:    "Currency",
			Fee:         "Fee",
			State:       "State",
		},
	}
}

var ErrUnknownProfile = fmt.Errorf("unknown bank profile")

func GetProfile(name string) (*Profile, error) {
	switch name {
	case "erste":
		return ErsteProfile(), nil
	case "revolut":
		return RevolutProfile(), nil
	}
	if p, err := loadCustomProfile(name); err == nil {
		return p, nil
	}
	return nil, fmt.Errorf("%w: %q (built-in: erste, revolut)", ErrUnknownProfile, name)
}

func DefaultProfileDir() string {
	if p := os.Getenv("LEDGER_PROFILE_DIR"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".ledger/profiles"
	}
	return filepath.Join(home, ".config", "ledger", "profiles")
}

func loadCustomProfile(name string) (*Profile, error) {
	dir := DefaultProfileDir()
	candidate := filepath.Join(dir, name+".toml")
	data, err := os.ReadFile(candidate)
	if err != nil {
		return nil, fmt.Errorf("read custom profile %s: %w", candidate, err)
	}
	var p Profile
	if err := toml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse custom profile %s: %w", candidate, err)
	}
	if p.Name == "" {
		p.Name = name
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

func (p *Profile) Validate() error {
	var missing []string
	if p.Name == "" {
		missing = append(missing, "name")
	}
	if p.Delimiter == "" {
		missing = append(missing, "delimiter")
	}
	if p.DateFormat == "" {
		missing = append(missing, "date_format")
	}
	if p.DecimalSep == "" {
		missing = append(missing, "decimal_sep")
	}
	if p.DefaultCurrency == "" {
		missing = append(missing, "default_currency")
	}
	if p.Columns.BookingDate == "" {
		missing = append(missing, "columns.booking_date")
	}
	if p.Columns.Amount == "" {
		missing = append(missing, "columns.amount")
	}
	if p.Columns.Description == "" {
		missing = append(missing, "columns.description")
	}
	if len(missing) > 0 {
		return fmt.Errorf("profile %q missing required fields: %s", p.Name, strings.Join(missing, ", "))
	}
	return nil
}