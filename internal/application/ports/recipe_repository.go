package ports

import (
	"context"

	"github.com/tomascosta29/Ledger/internal/domain/entities"
)

type RecipeRepository interface {
	LoadAll(ctx context.Context) ([]*entities.Recipe, error)
	LoadByName(ctx context.Context, name string) (*entities.Recipe, error)
	GetActiveName(ctx context.Context) (string, error)
	SetActiveName(ctx context.Context, name string) error
}
