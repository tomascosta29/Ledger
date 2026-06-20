package csv

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
)

type HashInput struct {
	ProfileName    string
	ProfileVersion int
	BookingDate    string
	AmountMinor    int64
	Currency       valueobjects.Currency
	PartnerName    string
	Description    string
}

func ComputeSourceHash(in HashInput) string {
	partner := normalizePartnerName(in.PartnerName)
	desc := strings.TrimSpace(in.Description)

	payload := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(in.ProfileName)),
		intToStr(int64(in.ProfileVersion)),
		in.BookingDate,
		intToStr(in.AmountMinor),
		string(in.Currency),
		strings.ToLower(partner),
		strings.ToLower(desc),
	}, "|")

	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func normalizePartnerName(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return ""
	}
	out := make([]string, len(fields))
	for i, f := range fields {
		if f == "" {
			continue
		}
		out[i] = strings.ToUpper(f[:1]) + strings.ToLower(f[1:])
	}
	return strings.Join(out, " ")
}

func intToStr(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}