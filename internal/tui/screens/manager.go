package screens

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/tui/hints"
	"github.com/tomascosta29/Ledger/internal/tui/styles"
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

func (m *Manager) View(width, height int) string {
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
	b.WriteString(m.renderHeader(width))
	b.WriteString("\n")
	b.WriteString(m.renderHeaderRule(width))
	b.WriteString("\n")

	// Reserve 2 lines for header + rule; render what fits in height.
	visibleRows := m.rows
	if height > 2 && len(visibleRows) > height-2 {
		visibleRows = visibleRows[:height-2]
	}
	for i, r := range visibleRows {
		b.WriteString(m.renderRow(i, r, width))
		b.WriteString("\n")
	}
	return b.String()
}

// renderHeader returns the styled column header row.
func (m *Manager) renderHeader(width int) string {
	idW := 5
	dateW := 10
	amountW := 13
	catW := 10
	descW := width - (1 + 1 + 3 + 1 + idW + 2 + dateW + 2 + amountW + 2 + catW + 2)
	if descW < 4 {
		descW = 4
	}
	h := fmt.Sprintf(" %s %s %s %s %s %s %s",
		strings.Repeat(" ", 1),
		strings.Repeat(" ", 3),
		padBoth("ID", idW),
		padBoth("DATE", dateW),
		padBoth("AMOUNT", amountW),
		padBoth("CAT", catW),
		"DESCRIPTION",
	)
	return styles.HeaderText.Render(truncateToWidth(h, width))
}

// renderHeaderRule returns the rule line under the header.
func (m *Manager) renderHeaderRule(width int) string {
	return styles.HeaderRule.Render(strings.Repeat(styles.RuleChar, width))
}

// renderRow returns one styled transaction row.
func (m *Manager) renderRow(i int, r managerRow, width int) string {
	isCursor := i == m.cursor
	isSel := m.selected[r.id]

	var rowStyle lipgloss.Style
	switch {
	case isCursor && isSel:
		rowStyle = styles.CursorSelectedRow
	case isCursor:
		rowStyle = styles.CursorRow
	case isSel:
		rowStyle = styles.SelectedRow
	}

	cursorChar := " "
	if isCursor {
		cursorChar = styles.CursorGlyph
	}
	selChar := styles.InactiveSelGlyph
	if isSel {
		selChar = styles.SelectionGlyph
	}

	idW := 5
	dateW := 10
	amountW := 13
	catW := 10
	descW := width - (1 + 1 + 3 + 1 + idW + 2 + dateW + 2 + amountW + 2 + catW + 2)
	if descW < 4 {
		descW = 4
	}

	id := fmt.Sprintf("%*d", idW, r.id)
	date := padRight(truncate(r.date, dateW), dateW)
	amount := styleAmount(r.amount, amountW)
	cat := padRight(truncate(coalesce(r.cat, "Unknown"), catW), catW)
	if r.cat == "" || r.cat == "Unknown" {
		cat = styles.UnknownCategory.Render(cat)
	}
	desc := truncate(r.desc, descW)

	row := cursorChar + " " + selChar + " " + id + "  " + date + "  " + amount + "  " + cat + "  " + desc
	return rowStyle.Render(row)
}

// styleAmount returns the amount colored by sign, right-aligned in
// the given width.
func styleAmount(s string, width int) string {
	if width < 1 {
		width = 1
	}
	s = truncate(s, width)
	var style lipgloss.Style
	switch {
	case strings.HasPrefix(s, "-"):
		style = styles.AmountOut
	case s == "" || (s[0] == '0' && !strings.ContainsAny(s, "123456789")):
		style = styles.AmountZero
	default:
		style = styles.AmountIn
	}
	pad := width - lipgloss.Width(s)
	if pad < 0 {
		pad = 0
	}
	return style.Render(strings.Repeat(" ", pad) + s)
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func padBoth(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	left := (n - len(s)) / 2
	right := n - len(s) - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

func coalesce(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// truncate returns s clipped to at most n runes, with an ellipsis if
// it was longer.
func truncate(s string, n int) string {
	if n < 1 {
		return ""
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	if n <= 1 {
		return string(s[:n])
	}
	// Trim rune-by-rune until it fits with the ellipsis suffix.
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))+lipgloss.Width(styles.EllipsisChar) > n {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + styles.EllipsisChar
}

// truncateToWidth clips a string to at most n visible cells.
func truncateToWidth(s string, n int) string {
	if n < 1 {
		return ""
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes)) > n {
		runes = runes[:len(runes)-1]
	}
	return string(runes)
}

func (m *Manager) Hints(width int) hints.FooterHints {
	// Mode-aware footer. Selected-count replaces the global key
	// hints when something is selected; filter and bulk modes
	// surface their own short hint set.
	if m.filterMode {
		return hints.FooterHints{
			Mode: "Filter",
			Keys: []hints.KeyHint{
				{Key: "Enter", Label: "apply"},
				{Key: "Esc", Label: "cancel"},
				{Key: "Bksp", Label: "edit"},
			},
		}
	}
	if m.bulkMode {
		label := "apply"
		switch m.bulkAction {
		case bulkCategorize:
			label = "Bulk: categorize"
		case bulkTag:
			label = "Bulk: tag"
		case bulkHide:
			label = "Bulk: hide"
		}
		return hints.FooterHints{
			Mode: label,
			Keys: []hints.KeyHint{
				{Key: "Enter", Label: "apply"},
				{Key: "Esc", Label: "cancel"},
			},
		}
	}
	if len(m.selected) > 0 {
		n := len(m.selected)
		return hints.FooterHints{
			Mode: "Normal",
			Keys: []hints.KeyHint{
				{Key: "[x]", Label: "toggle"},
				{Key: "C", Label: "cat", Count: n},
				{Key: "T", Label: "tag", Count: n},
				{Key: "H", Label: "hide", Count: n},
				{Key: "U", Label: "undo"},
				{Key: ":", Label: "clear"},
			},
		}
	}
	return hints.FooterHints{
		Mode: "Normal",
		Keys: []hints.KeyHint{
			{Key: "j/k", Label: "nav"},
			{Key: "/", Label: "filter"},
			{Key: "x", Label: "select"},
			{Key: "C", Label: "cat"},
			{Key: "T", Label: "tag"},
			{Key: "H", Label: "hide"},
			{Key: "U", Label: "undo"},
			{Key: "?", Label: "help"},
		},
	}
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
