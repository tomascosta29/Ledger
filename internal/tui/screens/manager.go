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
	bulkMode    bool
	bulkInput   string
	bulkAction  bulkActionKind
	selected    map[int64]bool
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

type bulkActionKind int

const (
	bulkNone bulkActionKind = iota
	bulkCategorize
	bulkTag
	bulkHide
)

func NewManager() *Manager {
	return &Manager{selected: make(map[int64]bool)}
}

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
		if m.bulkMode {
			return m.updateBulkInput(msg)
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
	case "x":
		if len(m.rows) > 0 && m.cursor < len(m.rows) {
			row := m.rows[m.cursor]
			if m.selected[row.id] {
				delete(m.selected, row.id)
			} else {
				m.selected[row.id] = true
			}
			m.statusMsg = fmt.Sprintf("%d selected", len(m.selected))
		}
	case "X":
		m.selected = make(map[int64]bool)
		m.statusMsg = "selection cleared"
	case "C":
		m.bulkMode = true
		m.bulkAction = bulkCategorize
		m.bulkInput = ""
		m.statusMsg = "categorize selection →"
	case "T":
		m.bulkMode = true
		m.bulkAction = bulkTag
		m.bulkInput = ""
		m.statusMsg = "tag selection +"
	case "H":
		m.applyBulkHide(ctx)
	case "U":
		m.applyBulkUndo(ctx)
	case "esc":
		m.filter = Filter{}
		m.reload(ctx)
		m.statusMsg = "filter cleared"
	}
	return m, nil
}

func (m *Manager) updateBulkInput(msg tea.KeyMsg) (Screen, tea.Cmd) {
	ctx := context.Background()
	switch msg.Type {
	case tea.KeyEsc:
		m.bulkMode = false
		m.bulkInput = ""
		m.bulkAction = bulkNone
		m.statusMsg = "cancelled"
	case tea.KeyEnter:
		action := m.bulkAction
		value := m.bulkInput
		m.bulkMode = false
		m.bulkInput = ""
		m.bulkAction = bulkNone
		switch action {
		case bulkCategorize:
			if value == "" {
				m.statusMsg = "empty category"
				return m, nil
			}
			if err := m.applyBulkCategorize(ctx, value); err != nil {
				m.statusMsg = "cat: " + err.Error()
			}
		case bulkTag:
			if value == "" {
				m.statusMsg = "empty tag"
				return m, nil
			}
			if err := m.applyBulkTag(ctx, value); err != nil {
				m.statusMsg = "tag: " + err.Error()
			}
		}
	case tea.KeyBackspace:
		if len(m.bulkInput) > 0 {
			m.bulkInput = m.bulkInput[:len(m.bulkInput)-1]
		}
	default:
		if len(msg.Runes) > 0 {
			m.bulkInput += string(msg.Runes)
		}
	}
	return m, nil
}

func (m *Manager) selectedIDs() []int64 {
	ids := make([]int64, 0, len(m.selected))
	for id := range m.selected {
		ids = append(ids, id)
	}
	return ids
}

func (m *Manager) applyBulkCategorize(ctx context.Context, cat string) error {
	ids := m.selectedIDs()
	if len(ids) == 0 {
		m.statusMsg = "no selection (press x on a row to select)"
		return nil
	}
	svc := annSvcFromDeps(m.deps)
	if err := svc.BulkCategorize(ctx, ids, cat, nil); err != nil {
		return err
	}
	m.statusMsg = fmt.Sprintf("categorized %d → %s", len(ids), cat)
	m.selected = make(map[int64]bool)
	m.reload(ctx)
	return nil
}

func (m *Manager) applyBulkTag(ctx context.Context, tag string) error {
	ids := m.selectedIDs()
	if len(ids) == 0 {
		m.statusMsg = "no selection (press x on a row to select)"
		return nil
	}
	svc := annSvcFromDeps(m.deps)
	if err := svc.BulkAddTags(ctx, ids, []string{tag}); err != nil {
		return err
	}
	m.statusMsg = fmt.Sprintf("tagged %d +%s", len(ids), tag)
	m.selected = make(map[int64]bool)
	m.reload(ctx)
	return nil
}

func (m *Manager) applyBulkHide(ctx context.Context) {
	ids := m.selectedIDs()
	if len(ids) == 0 {
		m.statusMsg = "no selection (press x on a row to select)"
		return
	}
	svc := annSvcFromDeps(m.deps)
	if err := svc.BulkSetHidden(ctx, ids, true); err != nil {
		m.statusMsg = "hide: " + err.Error()
		return
	}
	m.statusMsg = fmt.Sprintf("hidden %d", len(ids))
	m.selected = make(map[int64]bool)
	m.reload(ctx)
}

func (m *Manager) applyBulkUndo(ctx context.Context) {
	svc := annSvcFromDeps(m.deps)
	if err := svc.Undo(ctx); err != nil {
		m.statusMsg = "undo: " + err.Error()
		return
	}
	m.statusMsg = "undone"
	m.reload(ctx)
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
	if m.bulkMode {
		label := "?"
		switch m.bulkAction {
		case bulkCategorize:
			label = "categorize"
		case bulkTag:
			label = "tag"
		}
		return fmt.Sprintf("  %s on selection: %s_\n", label, m.bulkInput)
	}
	if len(m.rows) == 0 {
		return "  (no transactions — try `ledger import` or `ledger add`)\n"
	}
	var b strings.Builder
	b.WriteString("  ID    DATE         AMOUNT       CAT        DESCRIPTION\n")
	for i, r := range m.rows {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		sel := " "
		if m.selected[r.id] {
			sel = "x"
		}
		fmt.Fprintf(&b, "%s%s%-5d  %-11s  %-11s  %-9s  %s\n",
			cursor, sel, r.id, r.date, r.amount, r.cat, r.desc)
	}
	if len(m.selected) > 0 {
		fmt.Fprintf(&b, "  selected: %d    (C cat · T tag · H hide · U undo · X clear)\n", len(m.selected))
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
