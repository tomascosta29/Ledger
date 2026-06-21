package screens

import (
	"context"
	"database/sql"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
	"github.com/tomascosta29/Ledger/internal/tui/hints"
)

// Screen is a self-contained Bubble Tea model. The App owns one of
// these at a time and delegates Update + View to it.
//
// View receives the content area dimensions (after the chrome
// reserves space for header, sidebar, footer, and statusbar).
//
// Hints returns the mode-aware footer payload for the current state.
// The screen knows its own modes (normal, filter, bulk); the chrome
// just renders.
type Screen interface {
	Title() string
	Init(ctx context.Context, deps Deps) tea.Cmd
	Update(msg tea.Msg) (Screen, tea.Cmd)
	View(width, height int) string
	Hints(width int) hints.FooterHints
	StatusMsg() string
}

// Deps carries the things every screen needs (DB path, repositories,
// services). Set once at App init and passed in via Init.
type Deps struct {
	DB          *sql.DB
	DBPath      string
	TxRepo      ports.TransactionRepository
	TagRepo     ports.TagRepository
	BucketRepo  ports.BucketRepository
	AuditRepo   ports.AuditLogRepository
	BatchRepo   ports.ImportBatchRepository
	GroupRepo   ports.GroupRepository
	OverlayRepo ports.OverlayRepository
	OverlaySvc  ports.OverlayService
	BudgetSvc   BudgetQuerier
	RecipeSvc   RecipeQuerier
}

// BudgetQuerier is the slice of BucketService a screen actually needs.
// Defined here to keep the screens package independent of services.
type BudgetQuerier interface {
	SpendByMonth(ctx context.Context, month string) ([]ports.BucketSpend, error)
	UnassignedSpendByMonth(ctx context.Context, month string) ([]ports.BucketSpend, error)
}

// RecipeQuerier is the slice of RecipeService a screen needs.
type RecipeQuerier interface {
	LoadAll(ctx context.Context) ([]*entities.Recipe, error)
	GetActiveName(ctx context.Context) (string, error)
	SetActiveName(ctx context.Context, name string) error
}
