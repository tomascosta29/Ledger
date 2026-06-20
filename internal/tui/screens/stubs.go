package screens

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

type Categorizer struct {
	deps Deps
}

func NewCategorizer() *Categorizer { return &Categorizer{} }

func (c *Categorizer) Title() string { return "Categorizer" }

func (c *Categorizer) Init(ctx context.Context, deps Deps) tea.Cmd {
	c.deps = deps
	return nil
}

func (c *Categorizer) Update(msg tea.Msg) (Screen, tea.Cmd) { return c, nil }

func (c *Categorizer) View() string {
	return "  (categorizer screen — bulk-categorize Unknown transactions)\n" +
		"  use `ledger categorize <ids> --category X` for now\n"
}

type Linker struct{}

func NewLinker() *Linker { return &Linker{} }
func (l *Linker) Title() string { return "Linker" }
func (l *Linker) Init(ctx context.Context, deps Deps) tea.Cmd { return nil }
func (l *Linker) Update(msg tea.Msg) (Screen, tea.Cmd)        { return l, nil }
func (l *Linker) View() string {
	return "  (linker screen — group expenses with reimbursements)\n" +
		"  feature ships in the rules + linker milestone\n"
}

type Budget struct{}

func NewBudget() *Budget { return &Budget{} }
func (b *Budget) Title() string { return "Budget" }
func (b *Budget) Init(ctx context.Context, deps Deps) tea.Cmd { return nil }
func (b *Budget) Update(msg tea.Msg) (Screen, tea.Cmd)        { return b, nil }
func (b *Budget) View() string {
	return "  (budget screen — per-bucket allocation vs spend)\n" +
		"  use `ledger budget` for the CLI view\n"
}

type Recipes struct{}

func NewRecipes() *Recipes { return &Recipes{} }
func (r *Recipes) Title() string { return "Recipes" }
func (r *Recipes) Init(ctx context.Context, deps Deps) tea.Cmd { return nil }
func (r *Recipes) Update(msg tea.Msg) (Screen, tea.Cmd)        { return r, nil }
func (r *Recipes) View() string {
	return "  (recipes screen — list, edit, and pick active summary recipe)\n" +
		"  feature ships in the recipes milestone\n"
}
