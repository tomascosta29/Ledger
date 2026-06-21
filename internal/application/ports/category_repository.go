package ports

import (
	"context"

	"github.com/tomascosta29/Ledger/internal/domain/entities"
)

type CategoryRepository interface {
	List(ctx context.Context, includeArchived bool) ([]*entities.Category, error)
	GetByID(ctx context.Context, id int64) (*entities.Category, error)
	GetByName(ctx context.Context, name string) (*entities.Category, error)
}
