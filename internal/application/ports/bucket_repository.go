package ports

import (
	"context"

	"github.com/tomascosta29/Ledger/internal/domain/entities"
)

type BucketSpend struct {
	BucketID       int64
	BucketName     string
	Currency       string
	AllocatedMinor int64
	SpentMinor     int64
	Count          int64
}

type BucketRepository interface {
	Create(ctx context.Context, b *entities.Bucket) (int64, error)
	GetByID(ctx context.Context, id int64) (*entities.Bucket, error)
	GetByName(ctx context.Context, name string) (*entities.Bucket, error)
	List(ctx context.Context, includeArchived bool) ([]*entities.Bucket, error)
	Update(ctx context.Context, b *entities.Bucket) error
	Archive(ctx context.Context, id int64) error
	Delete(ctx context.Context, id int64) error
	CountAssignedTransactions(ctx context.Context, id int64) (int64, error)
	SpendByMonth(ctx context.Context, month string) ([]BucketSpend, error)
	UnassignedSpendByMonth(ctx context.Context, month string) ([]BucketSpend, error)
}
