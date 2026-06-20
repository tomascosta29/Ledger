package ports

import (
	"context"
	"database/sql"
	"time"

	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
)

type SourceKind string

const (
	SourceRaw                SourceKind = "raw"
	SourceSplitChild         SourceKind = "split_child"
	SourceSplitHeader        SourceKind = "split_header"
	SourceTransferGroup      SourceKind = "transfer_group"
	SourceReimbursementGroup SourceKind = "reimbursement_group"
)

type OverlayFilters struct {
	StartDate          *string
	EndDate            *string
	IsHidden           *bool
	ExcludeFromReports *bool
	Category           *string
	Categories         []string
	SourceKinds        []SourceKind
	GroupID            *int64
	ParentOverlayID    *int64
	RawTransactionID   *int64
	PartnerName        *string
	PartnerIBAN        *string
	DescriptionLike    *string
	AmountMinMinor     *int64
	AmountMaxMinor     *int64
	AmountSign         *string
	BucketID           *int64
}

type OverlaySortField string

const (
	OverlaySortByDate   OverlaySortField = "effective_date"
	OverlaySortByAmount OverlaySortField = "amount_minor"
	OverlaySortByID     OverlaySortField = "id"
)

type OverlayFindOptions struct {
	Filters OverlayFilters
	Sort    OverlaySortField
	Order   SortOrder
	Limit   int
}

type OverlayTransaction struct {
	ID                 int64
	EffectiveDate      string
	Amount             valueobjects.Money
	Description        string
	PartnerName        *string
	PartnerIBAN        *string
	Category           string
	BucketID           *int64
	Tags               string
	ParentOverlayID    *int64
	GroupID            *int64
	GroupRole          *string
	SourceKind         SourceKind
	RawTransactionID   *int64
	RawTransactionIDs  string
	ExcludeFromReports bool
	RefreshedAt        time.Time
}

type OverlayRepository interface {
	FindAll(ctx context.Context, opts OverlayFindOptions) ([]*OverlayTransaction, error)
	GetByID(ctx context.Context, id int64) (*OverlayTransaction, error)
	Count(ctx context.Context, filters OverlayFilters) (int64, error)
}

type OverlayService interface {
	RebuildWithTx(ctx context.Context, tx *sql.Tx) error
	Rebuild(ctx context.Context) error
}
