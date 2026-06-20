package ports

import (
	"context"

	"github.com/tomascosta29/Ledger/internal/domain/entities"
)

type RuleRepository interface {
	Create(ctx context.Context, r *entities.Rule) (int64, error)
	GetByID(ctx context.Context, id int64) (*entities.Rule, error)
	List(ctx context.Context, enabledOnly bool) ([]*entities.Rule, error)
	Delete(ctx context.Context, id int64) error
}
