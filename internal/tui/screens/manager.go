package screens

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tomascosta29/Ledger/internal/application/ports"
)

// Manager is the transaction list screen (filter DSL, j/k nav, ...).
type Manager struct {
	deps   Deps
	rows   []managerRow
	cursor int
	filter Filter
	filterInput string
	filterMode  bool
	statusMsg   string
}

type managerRow struct {
	id     int64
	date   string
	amount string
	cat    string
	desc   string
	rawID  *int64
}

func NewManager() *Manager { return &Manager{} }

func (m *Manager) Title() string { return "Manager" }

func (m *Manager) Init(ctx context.Context, deps Deps) tea.Cmd {
	m.deps = deps
	m.reload(ctx)
	return nil
}

func (m *Manager) reload(ctx context.Context) {
	opts := m.opts()
	rows, err := m.deps.OverlayRepo.FindAll(ctx, opts)
	if err != nil {
		m.statusMsg = "list: " + err.Error()
		return
	}
	m.rows = m.rows[:0]
	for _, r := range rows {
		m.rows = append(m.rows, managerRow{
			id:     r.ID,
			date:   r.EffectiveDate,
			amount: r.Amount.DecimalString() + " " + string(r.Amount.Currency),
			cat:    r.Category,
			desc:   r.Description,
			rawID:  r.RawTransactionID,
		})
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.statusMsg = fmt.Sprintf("showing %d", len(m.rows))
}

func (m *Manager) opts() ports.OverlayFindOptions {
	f := m.filter.Apply()
	return ports.OverlayFindOptions{
		Filters: f,
		Sort:    ports.OverlaySortByDate,
		Order:   ports.SortDesc,
		Limit:   200,
	}
}

func (m *Manager) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.filterMode {
			return m.updateFilterInput(msg)
		}
		return m.updateNormal(msg)
	}
	return m, nil
}

func (m *Manager) updateNormal(msg tea.KeyMsg) (Screen, tea.Cmd) {
	ctx := context.Background()
	switch msg.String() {
	case "j", "down":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "g":
		m.cursor = 0
	case "G":
		m.cursor = len(m.rows) - 1
	case "pgdown":
		m.cursor = min(m.cursor+10, len(m.rows)-1)
	case "pgup":
		m.cursor = max(m.cursor-10, 0)
	case "/":
		m.filterMode = true
		m.filterInput = ""
		m.statusMsg = "filter (enter to apply, esc to cancel)"
	case "esc":
		m.filter = Filter{}
		m.reload(ctx)
		m.statusMsg = "filter cleared"
	}
	return m, nil
}

func (m *Manager) updateFilterInput(msg tea.KeyMsg) (Screen, tea.Cmd) {
	ctx := context.Background()
	switch msg.Type {
	case tea.KeyEsc:
		m.filterMode = false
		m.filterInput = ""
		m.statusMsg = "filter cancelled"
	case tea.KeyEnter:
		f, err := Parse(m.filterInput)
		if err != nil {
			m.statusMsg = "filter: " + err.Error()
			return m, nil
		}
		m.filter = f
		m.filterMode = false
		m.filterInput = ""
		m.reload(ctx)
	case tea.KeyBackspace:
		if len(m.filterInput) > 0 {
			m.filterInput = m.filterInput[:len(m.filterInput)-1]
		}
	default:
		if len(msg.Runes) > 0 {
			m.filterInput += string(msg.Runes)
		}
	}
	return m, nil
}

func (m *Manager) View() string {
	if m.filterMode {
		return fmt.Sprintf("  filter: %s_\n", m.filterInput)
	}
	if len(m.rows) == 0 {
		return "  (no transactions — try `ledger import` or `ledger add`)\n"
	}
	var b strings.Builder
	b.WriteString("  ID    DATE         AMOUNT       CAT        DESCRIPTION\n")
	for i, r := range m.rows {
		marker := "  "
		if i == m.cursor {
			marker = "> "
		}
		fmt.Fprintf(&b, "%s%-5d  %-11s  %-11s  %-9s  %s\n",
			marker, r.id, r.date, r.amount, r.cat, r.desc)
	}
	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
