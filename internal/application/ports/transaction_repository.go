package ports

import (
	"context"
	"errors"

	"github.com/tomascosta29/Ledger/internal/domain/entities"
)

var ErrNotFound = errors.New("not found")

type SortField string

const (
	SortByDate   SortField = "effective_date"
	SortByAmount SortField = "amount_minor"
	SortByID     SortField = "id"
)

type SortOrder string

const (
	SortAsc  SortOrder = "asc"
	SortDesc SortOrder = "desc"
)

type TxFilters struct {
	StartDate           *string
	EndDate             *string
	IsHidden            *bool
	ExcludeFromReports  *bool
	Category            *string
	Categories          []string
	PartnerName         *string
	PartnerIBAN         *string
	AmountMinMinor      *int64
	AmountMaxMinor      *int64
	AmountSign          *string
	DescriptionLike     *string
	IDs                 []int64
	ExcludeIDs          []int64
}

type TxFindOptions struct {
	Filters TxFilters
	Sort    SortField
	Order   SortOrder
	Limit   int
}

type TransactionRepository interface {
	Insert(ctx context.Context, tx *entities.Transaction) (int64, error)
	InsertBatch(ctx context.Context, txs []*entities.Transaction) (insertedIDs []int64, err error)
	GetByID(ctx context.Context, id int64) (*entities.Transaction, error)
	GetBySourceHash(ctx context.Context, hash string) (*entities.Transaction, error)
	FindAll(ctx context.Context, opts TxFindOptions) ([]*entities.Transaction, error)
	UpdateFields(ctx context.Context, id int64, fields map[string]any) error
	SetHidden(ctx context.Context, id int64, hidden bool) error
	SetExcludeFromReports(ctx context.Context, id int64, exclude bool) error
	SetCategory(ctx context.Context, id int64, category string) error
	Count(ctx context.Context, filters TxFilters) (int64, error)

	SetCategoryDBTX(ctx context.Context, db DBTX, id int64, category string) error
	SetHiddenDBTX(ctx context.Context, db DBTX, id int64, hidden bool) error
}