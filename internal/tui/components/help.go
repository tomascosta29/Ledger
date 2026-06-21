package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Help struct{}

func NewHelp() Help { return Help{} }

func (h Help) View(width, height int) string {
	content := `LedgerPro TUI — keybindings

Global
  1 .. 4       jump to screen (Manager, Linker, Budget, Recipes)
  ?            this help
  q            quit
  ctrl+c       quit

Manager
  j / k        next / previous row
  g / G        first / last row
  pgup / pgdn  jump 10 rows
  /            enter filter DSL
  n            jump to next Unknown row
  x            toggle select
  c / b / t    set category / bucket / tag on cursor row
  C / B / T    same, on selection
  l            link selected rows as transfers
  H            hide selection
  U            undo last batch

Linker
  j / k        next / previous item
  Enter        link focused candidate

Budget
  Tab          switch focus (buckets ↔ unassigned)
  a            archive focused bucket
  n            new bucket

Recipes
  j / k        next / previous recipe
  u            use focused recipe (make it active)
  n            new recipe
  e            edit focused recipe

Press any key to close this help.`
	box := helpBoxStyle.Width(minInt(80, width-4)).Render(content)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var helpBoxStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("117")).
	Padding(1, 2).
	Foreground(lipgloss.Color("252"))

// silence unused import in older builds
var _ = strings.Repeat
