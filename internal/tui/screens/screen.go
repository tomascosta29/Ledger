package screens

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tomascosta29/Ledger/internal/application/ports"
)

// Screen is a self-contained Bubble Tea model. The App owns one of
// these at a time and delegates Update + View to it.
type Screen interface {
	Title() string
	Init(ctx context.Context, deps Deps) tea.Cmd
	Update(msg tea.Msg) (Screen, tea.Cmd)
	View() string
}

// Deps carries the things every screen needs (DB path, repositories,
// services). Set once at App init and passed in via Init.
type Deps struct {
	DBPath       string
	TxRepo       ports.TransactionRepository
	TagRepo      ports.TagRepository
	BucketRepo   ports.BucketRepository
	AuditRepo    ports.AuditLogRepository
	BatchRepo    ports.ImportBatchRepository
	OverlayRepo  ports.OverlayRepository
	OverlaySvc   ports.OverlayService
	BudgetSvc    BudgetQuerier
}

// BudgetQuerier is the slice of BucketService a screen actually needs.
// Defined here to keep the screens package independent of services.
type BudgetQuerier interface {
	SpendByMonth(ctx context.Context, month string) ([]ports.BucketSpend, error)
	UnassignedSpendByMonth(ctx context.Context, month string) ([]ports.BucketSpend, error)
}
