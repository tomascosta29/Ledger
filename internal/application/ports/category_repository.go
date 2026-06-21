package ports

import (
	"context"

	"github.com/tomascosta29/Ledger/internal/domain/entities"
)

type CategoryRepository interface {
	List(ctx context.Context, includeArchived bool) ([]*entities.Category, error)
	GetByID(ctx context.Context, id int64) (*entities.Category, error)
	GetByName(ctx context.Context, name string) (*entities.Category, error)
	Create(ctx context.Context, c *entities.Category) (int64, error)
	CreateDBTX(ctx context.Context, db DBTX, c *entities.Category) (int64, error)
	Rename(ctx context.Context, id int64, newName string) error
	RenameDBTX(ctx context.Context, db DBTX, id int64, newName string) error
	Archive(ctx context.Context, id int64) error
	ArchiveDBTX(ctx context.Context, db DBTX, id int64) error
}
