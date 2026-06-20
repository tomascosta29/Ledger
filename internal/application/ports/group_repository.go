package ports

import (
	"context"

	"github.com/tomascosta29/Ledger/internal/domain/entities"
)

type GroupRepository interface {
	CreateGroup(ctx context.Context, g *entities.TransactionGroup) (int64, error)
	GetGroup(ctx context.Context, id int64) (*entities.TransactionGroup, error)
	AddMember(ctx context.Context, groupID, txID int64, role string) error
	ListMembers(ctx context.Context, groupID int64) ([]*entities.GroupMember, error)
	RemoveMember(ctx context.Context, groupID, txID int64) error
	ListGroups(ctx context.Context) ([]*entities.TransactionGroup, error)
}
