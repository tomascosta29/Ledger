package ports

import "context"

type TagRepository interface {
	Add(ctx context.Context, transactionID int64, tag string) error
	Remove(ctx context.Context, transactionID int64, tag string) error
	ListByTransaction(ctx context.Context, transactionID int64) ([]string, error)
	ListByTag(ctx context.Context, tag string) ([]int64, error)

	AddDBTX(ctx context.Context, db DBTX, transactionID int64, tag string) error
	RemoveDBTX(ctx context.Context, db DBTX, transactionID int64, tag string) error
	Clear(ctx context.Context, transactionID int64) error
	ClearDBTX(ctx context.Context, db DBTX, transactionID int64) error
}
