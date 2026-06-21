package screens

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/application/services"
	"github.com/tomascosta29/Ledger/internal/tui/hints"
	"github.com/tomascosta29/Ledger/internal/tui/styles"
)

// _ ensures ports is referenced even if the only use is in managerRow's
// SourceKind field, so the import is not flagged by goimports.
var _ ports.SourceKind

// Manager is the transaction list screen (filter DSL, j/k nav, ...).
// Single screen after v1.3 — absorbs Categorizer's single-row
// annotation (c/b/t) plus bulk on selection (C/B/T) and a quick
// jump-to-next-Unknown (n) for the triage loop.
type Manager struct {
	deps   Deps
	rows   []managerRow
	cursor int
	filter Filter
	filterInput string
	filterMode  bool

	// Unified annotation input state. inputMode != inputNone means
	// the operator is typing a category / bucket / tag value;
	// inputScope tells us whether Enter applies to the cursor row
	// or to the multi-selection. Replaces the older bulkMode pair.
	inputMode  inputKind
	inputScope inputScope
	input      string

	selected  map[int64]bool
	statusMsg string
}

type managerRow struct {
	id        int64
	date      string
	amount    string
	cat       string
	desc      string
	rawID     *int64
	linked    bool             // true when source_kind == "group"
	groupID   *int64           // non-nil if linked
	sourceKind ports.SourceKind // raw, split_child, split_header, group
}

type inputKind int

const (
	inputNone inputKind = iota
	inputCategory
	inputBucket
	inputTag
)

func (k inputKind) label() string {
	switch k {
	case inputCategory:
		return "category"
	case inputBucket:
		return "bucket"
	case inputTag:
		return "tag"
	}
	return ""
}

type inputScope int

const (
	scopeCursor inputScope = iota
	scopeSelection
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
			id:         r.ID,
			date:       r.EffectiveDate,
			amount:     r.Amount.DecimalString() + " " + string(r.Amount.Currency),
			cat:        r.Category,
			desc:       r.Description,
			rawID:      r.RawTransactionID,
			linked:     r.SourceKind == ports.SourceGroup,
			groupID:    r.GroupID,
			sourceKind: r.SourceKind,
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
	if msg, ok := msg.(tea.KeyMsg); ok {
		if m.filterMode {
			return m.updateFilterInput(msg)
		}
		if m.inputMode != inputNone {
			return m.updateInput(msg)
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
	case "n":
		m.jumpToNextUnknown()
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
	case "c":
		if m.cursor < len(m.rows) {
			m.inputMode = inputCategory
			m.inputScope = scopeCursor
			m.input = ""
			m.statusMsg = "category on cursor →"
		}
	case "b":
		if m.cursor < len(m.rows) {
			m.inputMode = inputBucket
			m.inputScope = scopeCursor
			m.input = ""
			m.statusMsg = "bucket on cursor →"
		}
	case "t":
		if m.cursor < len(m.rows) {
			m.inputMode = inputTag
			m.inputScope = scopeCursor
			m.input = ""
			m.statusMsg = "tag on cursor →"
		}
	case "C":
		if len(m.selected) == 0 {
			m.statusMsg = "no selection (press x on rows)"
			return m, nil
		}
		m.inputMode = inputCategory
		m.inputScope = scopeSelection
		m.input = ""
		m.statusMsg = fmt.Sprintf("category on %d →", len(m.selected))
	case "B":
		if len(m.selected) == 0 {
			m.statusMsg = "no selection (press x on rows)"
			return m, nil
		}
		m.inputMode = inputBucket
		m.inputScope = scopeSelection
		m.input = ""
		m.statusMsg = fmt.Sprintf("bucket on %d →", len(m.selected))
	case "T":
		if len(m.selected) == 0 {
			m.statusMsg = "no selection (press x on rows)"
			return m, nil
		}
		m.inputMode = inputTag
		m.inputScope = scopeSelection
		m.input = ""
		m.statusMsg = fmt.Sprintf("tag on %d +", len(m.selected))
	case "H":
		m.applyBulkHide(ctx)
	case "l":
		return m.linkSelected(ctx)
	case "U":
		m.applyBulkUndo(ctx)
	case "esc":
		m.filter = Filter{}
		m.reload(ctx)
		m.statusMsg = "filter cleared"
	}
	return m, nil
}

// updateInput handles keystrokes while an annotation prompt is open.
// Esc cancels, Enter applies, runes and backspace edit the buffer.
func (m *Manager) updateInput(msg tea.KeyMsg) (Screen, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.inputMode = inputNone
		m.input = ""
		m.statusMsg = "cancelled"
	case tea.KeyEnter:
		mode := m.inputMode
		scope := m.inputScope
		value := m.input
		m.inputMode = inputNone
		m.input = ""
		return m.applyInput(context.Background(), mode, scope, value)
	case tea.KeyBackspace:
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	default:
		if len(msg.Runes) > 0 {
			m.input += string(msg.Runes)
		}
	}
	return m, nil
}

// jumpToNextUnknown moves the cursor to the next row with empty
// category (Category == "" means the overlay reports NULL as "").
// Wraps around once; reports if no Unknown rows remain.
func (m *Manager) jumpToNextUnknown() {
	for i := m.cursor + 1; i < len(m.rows); i++ {
		if m.rows[i].cat == "" {
			m.cursor = i
			return
		}
	}
	for i := 0; i <= m.cursor; i++ {
		if m.rows[i].cat == "" {
			m.cursor = i
			return
		}
	}
	m.statusMsg = "no Unknown rows"
}

// applyInput dispatches the typed value to the right service call
// based on (kind, scope). Bulk methods reuse the same code path
// because AnnotationService methods accept a slice of IDs.
func (m *Manager) applyInput(ctx context.Context, mode inputKind, scope inputScope, value string) (Screen, tea.Cmd) {
	if value == "" {
		m.statusMsg = "empty " + mode.label()
		return m, nil
	}
	var ids []int64
	switch scope {
	case scopeCursor:
		if m.cursor >= len(m.rows) {
			m.statusMsg = "no row"
			return m, nil
		}
		ids = []int64{m.rows[m.cursor].id}
	case scopeSelection:
		ids = m.selectedIDs()
		if len(ids) == 0 {
			m.statusMsg = "no selection"
			return m, nil
		}
	}
	svc := annSvcFromDeps(m.deps)
	var err error
	switch mode {
	case inputCategory:
		err = svc.BulkCategorize(ctx, ids, value, nil)
	case inputBucket:
		err = svc.BulkSetBucket(ctx, ids, value)
	case inputTag:
		err = svc.BulkAddTags(ctx, ids, []string{value})
	}
	if err != nil {
		m.statusMsg = mode.label() + ": " + err.Error()
		return m, nil
	}
	switch scope {
	case scopeCursor:
		m.statusMsg = fmt.Sprintf("%s %d → %s", mode.label(), ids[0], value)
	case scopeSelection:
		m.statusMsg = fmt.Sprintf("%s %d → %s", mode.label(), len(ids), value)
		m.selected = make(map[int64]bool)
	}
	m.reload(ctx)
	return m, nil
}

func (m *Manager) selectedIDs() []int64 {
	ids := make([]int64, 0, len(m.selected))
	for id := range m.selected {
		ids = append(ids, id)
	}
	return ids
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

// linkSelected runs transfer detection on the selected rows and
// confirms any pair where both the out-tx and in-tx are in the
// selection. Returns the Screen for the bubbletea return contract;
// statusMsg carries the result and distinguishes the three outcomes
// the operator can observe:
//   - at least one pair was confirmed ("linked N pair(s)")
//   - some candidates matched the selection but Confirm failed
//     ("linked N, M failed: <err>")
//   - no candidates matched the selection at all
//     ("no transfer candidate between selected rows")
// so "linked 0" is never confused with "linked some".
func (m *Manager) linkSelected(ctx context.Context) (Screen, tea.Cmd) {
	if len(m.selected) < 2 {
		m.statusMsg = "link: select 2+ rows with x first"
		return m, nil
	}
	svc := services.NewTransferService(services.TransferDetectionDeps{
		TxRepo:    m.deps.TxRepo,
		GroupRepo: m.deps.GroupRepo,
		AuditRepo: m.deps.AuditRepo,
		OverlaySvc: m.deps.OverlaySvc,
	})
	cands, err := svc.Detect(ctx)
	if err != nil {
		m.statusMsg = "link detect: " + err.Error()
		return m, nil
	}
	linked := 0
	failed := 0
	matched := 0
	var lastErr error
	for _, c := range cands {
		if !m.selected[c.OutID] || !m.selected[c.InID] {
			continue
		}
		matched++
		if _, err := svc.Confirm(ctx, services.TransferCandidate{
			OutID: c.OutID,
			InID:  c.InID,
		}); err != nil {
			failed++
			lastErr = err
			continue
		}
		linked++
	}
	m.selected = make(map[int64]bool)
	switch {
	case linked > 0 && failed == 0:
		m.statusMsg = fmt.Sprintf("linked %d pair(s)", linked)
	case linked > 0 && failed > 0:
		m.statusMsg = fmt.Sprintf("linked %d, %d failed: %s", linked, failed, lastErr.Error())
	case linked == 0 && failed > 0:
		m.statusMsg = fmt.Sprintf("link failed: %s", lastErr.Error())
	case matched == 0:
		m.statusMsg = "no transfer candidate between selected rows"
	default:
		m.statusMsg = "no pairs linked"
	}
	m.reload(ctx)
	return m, nil
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
	if m.inputMode != inputNone {
		scope := "cursor"
		if m.inputScope == scopeSelection {
			scope = fmt.Sprintf("%d selected", len(m.selected))
		}
		return fmt.Sprintf("  %s on %s: %s_\n", m.inputMode.label(), scope, m.input)
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
	linkChar := styles.InactiveLinkedGlyph
	if r.linked {
		linkChar = styles.LinkedGlyph
	}

	idW := 5
	dateW := 10
	amountW := 13
	catW := 10
	// Column widths: 1 link + 1 space + 1 cursor + 1 space +
	// 3 sel + 1 space + 5 id + 2 + 10 date + 2 + 13 amount +
	// 2 + 10 cat + 2 + desc.
	descW := width - (1 + 1 + 1 + 1 + 3 + 1 + idW + 2 + dateW + 2 + amountW + 2 + catW + 2)
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

	// The link glyph sits at the leftmost column so it stays put
	// when the cursor moves. Color it Accent; the row's own style
	// (cursor/selection) will take precedence in the terminal
	// because we render glyph inside the row string.
	linkStyled := styles.LinkedGlyphStyle.Render(linkChar)
	row := linkStyled + " " + cursorChar + " " + selChar + " " + id + "  " + date + "  " + amount + "  " + cat + "  " + desc
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
	// Mode-aware footer. Filter and annotation prompts surface
	// their own short hint set. With nothing selected, hints lead
	// with the single-row keys (c/b/t + n) which are the primary
	// triage loop; bulk keys (C/B/T) come later and truncate first
	// at narrow widths.
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
	if m.inputMode != inputNone {
		label := m.inputMode.label()
		switch m.inputScope {
		case scopeCursor:
			label = "Input: " + label
		case scopeSelection:
			label = "Bulk: " + label
		}
		return hints.FooterHints{
			Mode: label,
			Keys: []hints.KeyHint{
				{Key: "Enter", Label: "apply"},
				{Key: "Esc", Label: "cancel"},
				{Key: "Bksp", Label: "edit"},
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
				{Key: "B", Label: "bkt", Count: n},
				{Key: "T", Label: "tag", Count: n},
				{Key: "l", Label: "link", Count: n},
				{Key: "H", Label: "hide", Count: n},
				{Key: "U", Label: "undo"},
				{Key: "X", Label: "clear"},
			},
		}
	}
	return hints.FooterHints{
		Mode: "Normal",
		Keys: []hints.KeyHint{
			{Key: "j/k", Label: "nav"},
			{Key: "n", Label: "unk"},
			{Key: "c", Label: "cat"},
			{Key: "t", Label: "tag"},
			{Key: "b", Label: "bkt"},
			{Key: "l", Label: "link"},
			{Key: "/", Label: "filter"},
			{Key: "x", Label: "select"},
			{Key: "C", Label: "cat·N"},
			{Key: "T", Label: "tag·N"},
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
