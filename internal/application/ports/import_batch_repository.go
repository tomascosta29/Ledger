package ports

import (
	"context"

	"github.com/tomascosta29/Ledger/internal/domain/entities"
)

type ImportBatchRepository interface {
	Create(ctx context.Context, batch *entities.ImportBatch) (int64, error)
	UpdateCounts(ctx context.Context, id int64, inserted, skipped int) error
	GetByID(ctx context.Context, id int64) (*entities.ImportBatch, error)
	Recent(ctx context.Context, limit int) ([]*entities.ImportBatch, error)
	Delete(ctx context.Context, id int64) error
	DeleteDBTX(ctx context.Context, db DBTX, id int64) error
}
