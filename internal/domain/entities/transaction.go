package entities

import (
	"time"

	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
)

type Transaction struct {
	ID                 int64
	EffectiveDate      string
	Amount             valueobjects.Money
	Description        string
	PartnerName        *string
	PartnerIBAN        *string
	ImportBatchID      *int64
	ParentTxnID        *int64
	SourceHash         string
	RawData            []byte
	RawDescription     *string
	CategoryID         *int64
	BucketID           *int64
	ExcludeFromReports bool
	IsHidden           bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (t Transaction) IsExpense() bool { return t.Amount.IsNegative() }
func (t Transaction) IsIncome() bool  { return t.Amount.IsPositive() }

func (t Transaction) NetAmount(reimbursementAdj int64) valueobjects.Money {
	net, _ := t.Amount.Add(valueobjects.MustNew(reimbursementAdj, t.Amount.Currency))
	return net
}
