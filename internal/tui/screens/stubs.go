package screens

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/application/services"
	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
	"github.com/tomascosta29/Ledger/internal/tui/hints"
	"github.com/tomascosta29/Ledger/internal/tui/styles"
)

type Categorizer struct {
	deps Deps
	rows  []catRow
	cursor int
	input   string
	inputMode inputKind
	statusMsg string
}

type catRow struct {
	id     int64
	date   string
	amount string
	desc   string
	rawID  *int64
}

type inputKind int

const (
	inputNone inputKind = iota
	inputCategory
	inputBucket
	inputTag
)

func NewCategorizer() *Categorizer { return &Categorizer{} }

func (c *Categorizer) Title() string { return "Categorizer" }

func (c *Categorizer) Init(ctx context.Context, deps Deps) tea.Cmd {
	c.deps = deps
	c.reload(ctx)
	return nil
}

func (c *Categorizer) reload(ctx context.Context) {
	// "Unknown" is a system state, not a value: the overlay rebuild
	// denormalizes category_id IS NULL as the empty string. Filter on
	// '' to surface uncategorized raw transactions.
	cat := ""
	opts := ports.OverlayFindOptions{
		Filters: ports.OverlayFilters{
			SourceKinds: []ports.SourceKind{ports.SourceRaw},
			Category:    &cat,
		},
		Sort:  ports.OverlaySortByDate,
		Order: ports.SortDesc,
		Limit: 500,
	}
	rows, err := c.deps.OverlayRepo.FindAll(ctx, opts)
	if err != nil {
		c.statusMsg = "load: " + err.Error()
		return
	}
	c.rows = c.rows[:0]
	for _, r := range rows {
		c.rows = append(c.rows, catRow{
			id:     r.ID,
			date:   r.EffectiveDate,
			amount: r.Amount.DecimalString() + " " + string(r.Amount.Currency),
			desc:   r.Description,
			rawID:  r.RawTransactionID,
		})
	}
	if c.cursor >= len(c.rows) {
		c.cursor = len(c.rows) - 1
	}
	if c.cursor < 0 {
		c.cursor = 0
	}
	c.statusMsg = fmt.Sprintf("%d unknown remaining", len(c.rows))
}

func (c *Categorizer) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if c.inputMode != inputNone {
			return c.updateInput(msg)
		}
		return c.updateNormal(msg)
	}
	return c, nil
}

func (c *Categorizer) updateNormal(msg tea.KeyMsg) (Screen, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if c.cursor < len(c.rows)-1 {
			c.cursor++
		}
	case "k", "up":
		if c.cursor > 0 {
			c.cursor--
		}
	case "g":
		c.cursor = 0
	case "G":
		c.cursor = len(c.rows) - 1
	case "c":
		c.inputMode = inputCategory
		c.input = ""
		c.statusMsg = "category:"
	case "b":
		c.inputMode = inputBucket
		c.input = ""
		c.statusMsg = "bucket:"
	case "t":
		c.inputMode = inputTag
		c.input = ""
		c.statusMsg = "tag:"
	}
	return c, nil
}

func (c *Categorizer) updateInput(msg tea.KeyMsg) (Screen, tea.Cmd) {
	ctx := context.Background()
	switch msg.Type {
	case tea.KeyEsc:
		c.inputMode = inputNone
		c.input = ""
		c.statusMsg = "cancelled"
	case tea.KeyEnter:
		action := c.inputMode
		value := c.input
		c.inputMode = inputNone
		c.input = ""
		if len(c.rows) == 0 || c.cursor >= len(c.rows) {
			c.statusMsg = "no row to apply to"
			return c, nil
		}
		rawID := c.rows[c.cursor].rawID
		if rawID == nil {
			c.statusMsg = "row has no raw transaction id"
			return c, nil
		}
		svc := annSvcFromDeps(c.deps)
		switch action {
		case inputCategory:
			if value == "" {
				c.statusMsg = "empty category"
				return c, nil
			}
			if err := svc.Categorize(ctx, *rawID, value, nil); err != nil {
				c.statusMsg = "categorize: " + err.Error()
				return c, nil
			}
			c.statusMsg = "categorized → " + value
		case inputBucket:
			if value == "" {
				c.statusMsg = "empty bucket"
				return c, nil
			}
			bucketName := value
			txn, err := c.deps.TxRepo.GetByID(ctx, *rawID)
			if err != nil {
				c.statusMsg = "load: " + err.Error()
				return c, nil
			}
			_ = txn
			if err := svc.Categorize(ctx, *rawID, "Unknown", &bucketName); err != nil {
				c.statusMsg = "bucket: " + err.Error()
				return c, nil
			}
			c.statusMsg = "bucket → " + value
		case inputTag:
			if value == "" {
				c.statusMsg = "empty tag"
				return c, nil
			}
			if err := svc.BulkAddTags(ctx, []int64{*rawID}, []string{value}); err != nil {
				c.statusMsg = "tag: " + err.Error()
				return c, nil
			}
			c.statusMsg = "tagged +" + value
		}
		c.reload(ctx)
	case tea.KeyBackspace:
		if len(c.input) > 0 {
			c.input = c.input[:len(c.input)-1]
		}
	default:
		if len(msg.Runes) > 0 {
			c.input += string(msg.Runes)
		}
	}
	return c, nil
}

func (c *Categorizer) View(width, height int) string {
	if c.inputMode != inputNone {
		label := ""
		switch c.inputMode {
		case inputCategory:
			label = "category"
		case inputBucket:
			label = "bucket"
		case inputTag:
			label = "tag"
		}
		prompt := fmt.Sprintf("  %s: %s_", label, c.input)
		if width > 0 {
			return styles.FooterMode.Render(label) + styles.FooterKey.Render(": "+c.input+"_") + "\n"
		}
		return prompt + "\n"
	}
	if len(c.rows) == 0 {
		return "  (no Unknown transactions — try Manager or Categorizer with filter)\n"
	}

	var b strings.Builder
	b.WriteString(c.renderHeader(width))
	b.WriteString("\n")
	b.WriteString(styles.HeaderRule.Render(strings.Repeat(styles.RuleChar, width)))
	b.WriteString("\n")

	visible := c.rows
	if height > 2 && len(visible) > height-2 {
		visible = visible[:height-2]
	}
	for i, r := range visible {
		b.WriteString(c.renderRow(i, r, width))
		b.WriteString("\n")
	}
	return b.String()
}

func (c *Categorizer) renderHeader(width int) string {
	idW := 5
	dateW := 10
	amountW := 13
	descW := width - (1 + 1 + 5 + 2 + dateW + 2 + amountW + 2)
	if descW < 4 {
		descW = 4
	}
	h := fmt.Sprintf(" %s %s %s %s %s",
		strings.Repeat(" ", 1),                  // cursor col placeholder
		padBoth("ID", idW),
		padBoth("DATE", dateW),
		padBoth("AMOUNT", amountW),
		"DESCRIPTION",
	)
	return styles.HeaderText.Render(truncateToWidth(h, width))
}

func (c *Categorizer) renderRow(i int, r catRow, width int) string {
	isCursor := i == c.cursor
	rowStyle := lipgloss.Style{}
	if isCursor {
		rowStyle = styles.CursorRow
	}
	cursorChar := " "
	if isCursor {
		cursorChar = styles.CursorGlyph
	}
	idW := 5
	dateW := 10
	amountW := 13
	descW := width - (1 + 1 + idW + 2 + dateW + 2 + amountW + 2)
	if descW < 4 {
		descW = 4
	}
	id := fmt.Sprintf("%*d", idW, r.id)
	date := padRight(truncate(r.date, dateW), dateW)
	amount := styleAmount(r.amount, amountW)
	desc := truncate(r.desc, descW)
	row := cursorChar + " " + id + "  " + date + "  " + amount + "  " + desc
	return rowStyle.Render(row)
}

func (c *Categorizer) Hints(width int) hints.FooterHints {
	if c.inputMode != inputNone {
		return hints.FooterHints{
			Mode: "Input",
			Keys: []hints.KeyHint{
				{Key: "Enter", Label: "apply"},
				{Key: "Esc", Label: "cancel"},
				{Key: "Bksp", Label: "edit"},
			},
		}
	}
	return hints.FooterHints{
		Mode: "Normal",
		Keys: []hints.KeyHint{
			{Key: "j/k", Label: "nav"},
			{Key: "c", Label: "cat"},
			{Key: "b", Label: "bucket"},
			{Key: "t", Label: "tag"},
			{Key: "Enter", Label: "apply"},
			{Key: "?", Label: "help"},
		},
	}
}

type Linker struct {
	deps      Deps
	cands     []linkerCand
	groups    []linkerGroup
	cursor    int
	focus     int // 0 = candidates, 1 = groups
	statusMsg string
}

type linkerCand struct {
	score  int
	outID  int64
	inID   int64
	outTxt string
	inTxt  string
}

type linkerGroup struct {
	id   int64
	note string
}

func NewLinker() *Linker { return &Linker{} }

func (l *Linker) Title() string { return "Linker" }

func (l *Linker) Init(ctx context.Context, deps Deps) tea.Cmd {
	l.deps = deps
	l.reload(ctx)
	return nil
}

func (l *Linker) reload(ctx context.Context) {
	svc := services.NewTransferService(services.TransferDetectionDeps{
		TxRepo:    l.deps.TxRepo,
		GroupRepo: l.deps.GroupRepo,
		AuditRepo: l.deps.AuditRepo,
		OverlaySvc: l.deps.OverlaySvc,
	})
	cands, err := svc.Detect(ctx)
	if err != nil {
		l.statusMsg = "detect: " + err.Error()
		return
	}
	l.cands = l.cands[:0]
	for _, c := range cands {
		l.cands = append(l.cands, linkerCand{
			score:  c.Score,
			outID:  c.OutID,
			inID:   c.InID,
			outTxt: fmt.Sprintf("%d  %s  %s  %d", c.OutID, c.OutDate, c.OutPartner, c.OutAmount),
			inTxt:  fmt.Sprintf("%d  %s  %s  %d", c.InID, c.InDate, c.InPartner, c.InAmount),
		})
	}
	groupRepo := linkerGroupRepo(l.deps)
	if groupRepo != nil {
		all, err := groupRepo.ListGroups(ctx)
		if err == nil {
				l.groups = l.groups[:0]
				for _, g := range all {
					note := g.Name
					if note == "" {
						note = fmt.Sprintf("%d", g.ID)
					}
					l.groups = append(l.groups, linkerGroup{id: g.ID, note: note})
				}
		}
	}
	if l.cursor >= len(l.cands)+len(l.groups) {
		l.cursor = len(l.cands) + len(l.groups) - 1
	}
	if l.cursor < 0 {
		l.cursor = 0
	}
	l.statusMsg = fmt.Sprintf("%d candidates · %d groups", len(l.cands), len(l.groups))
}

func (l *Linker) Update(msg tea.Msg) (Screen, tea.Cmd) {
	ctx := context.Background()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		total := len(l.cands) + len(l.groups)
		switch msg.String() {
		case "j", "down":
			if l.cursor < total-1 {
				l.cursor++
			}
		case "k", "up":
			if l.cursor > 0 {
				l.cursor--
			}
		case "enter":
			if l.cursor < len(l.cands) {
				c := l.cands[l.cursor]
				svc := services.NewTransferService(services.TransferDetectionDeps{
					TxRepo: l.deps.TxRepo, GroupRepo: l.deps.GroupRepo,
					AuditRepo: l.deps.AuditRepo, OverlaySvc: l.deps.OverlaySvc,
				})
				if _, err := svc.Confirm(ctx, services.TransferCandidate{
					OutID: c.outID, InID: c.inID,
				}); err != nil {
					l.statusMsg = "confirm: " + err.Error()
				} else {
					l.statusMsg = fmt.Sprintf("linked %d ↔ %d", c.outID, c.inID)
				}
				l.reload(ctx)
			}
		}
	}
	return l, nil
}

func (l *Linker) View(width, height int) string {
	var sb strings.Builder

	sb.WriteString(styles.HeaderText.Render("Candidates"))
	sb.WriteString(styles.HeaderRule.Render(" " + strings.Repeat(styles.RuleChar, maxInt(width-12, 4))))
	sb.WriteString("\n")

	if len(l.cands) == 0 {
		sb.WriteString(styles.UnknownCategory.Render("  (no candidates)"))
		sb.WriteString("\n")
	} else {
		for i, c := range l.cands {
			sb.WriteString(l.renderCandidate(i, c, width))
			sb.WriteString("\n")
		}
	}

	if len(l.groups) > 0 {
		sb.WriteString("\n")
		sb.WriteString(styles.HeaderText.Render("Groups"))
		sb.WriteString(styles.HeaderRule.Render(" " + strings.Repeat(styles.RuleChar, maxInt(width-7, 4))))
		sb.WriteString("\n")
		for i, g := range l.groups {
			sb.WriteString(l.renderGroup(i, g, width))
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func (l *Linker) renderCandidate(i int, c linkerCand, width int) string {
	isCursor := i == l.cursor
	rowStyle := lipgloss.Style{}
	if isCursor {
		rowStyle = styles.CursorRow
	}
	cursorChar := "  "
	if isCursor {
		cursorChar = styles.CursorGlyph + " "
	}
	score := fmt.Sprintf("score %d", c.score)
	scoreStyled := styles.AmountIn.Render(score)
	rest := fmt.Sprintf("out=%s  |  in=%s", c.outTxt, c.inTxt)
	if width > 0 {
		rest = truncate(rest, maxInt(width-lipgloss.Width(cursorChar)-lipgloss.Width(scoreStyled)-2, 4))
	}
	row := cursorChar + scoreStyled + "  " + rest
	return rowStyle.Render(row)
}

func (l *Linker) renderGroup(i int, g linkerGroup, width int) string {
	absIdx := len(l.cands) + i
	isCursor := absIdx == l.cursor
	rowStyle := lipgloss.Style{}
	if isCursor {
		rowStyle = styles.CursorRow
	}
	cursorChar := "  "
	if isCursor {
		cursorChar = styles.CursorGlyph + " "
	}
	id := fmt.Sprintf("#%d", g.id)
	rest := g.note
	if width > 0 {
		rest = truncate(rest, maxInt(width-lipgloss.Width(cursorChar)-lipgloss.Width(id)-2, 4))
	}
	row := cursorChar + id + "  " + rest
	return rowStyle.Render(row)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (l *Linker) Hints(width int) hints.FooterHints {
	return hints.FooterHints{
		Mode: "Normal",
		Keys: []hints.KeyHint{
			{Key: "j/k", Label: "nav"},
			{Key: "Enter", Label: "link"},
			{Key: "?", Label: "help"},
		},
	}
}

type Budget struct {
	deps      Deps
	month     string
	spends    []ports.BucketSpend
	unassigned []ports.BucketSpend
	statusMsg string
}

func NewBudget() *Budget { return &Budget{} }

func (b *Budget) Title() string { return "Budget" }

func (b *Budget) Init(ctx context.Context, deps Deps) tea.Cmd {
	b.deps = deps
	b.month = time.Now().UTC().Format("2006-01")
	b.reload(ctx)
	return nil
}

func (b *Budget) reload(ctx context.Context) {
	spends, err := b.deps.BudgetSvc.SpendByMonth(ctx, b.month)
	if err != nil {
		b.statusMsg = "spend: " + err.Error()
		return
	}
	unassigned, err := b.deps.BudgetSvc.UnassignedSpendByMonth(ctx, b.month)
	if err != nil {
		b.statusMsg = "unassigned: " + err.Error()
		return
	}
	b.spends = spends
	b.unassigned = unassigned
	b.statusMsg = fmt.Sprintf("%d buckets · %d unassigned currencies", len(spends), len(unassigned))
}

func (b *Budget) Update(msg tea.Msg) (Screen, tea.Cmd) {
	ctx := context.Background()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "n":
			// next month
			t, err := time.Parse("2006-01", b.month)
			if err != nil {
				b.statusMsg = "bad month " + b.month
				return b, nil
			}
			t = t.AddDate(0, 1, 0)
			b.month = t.Format("2006-01")
			b.reload(ctx)
		case "p":
			t, err := time.Parse("2006-01", b.month)
			if err != nil {
				b.statusMsg = "bad month " + b.month
				return b, nil
			}
			t = t.AddDate(0, -1, 0)
			b.month = t.Format("2006-01")
			b.reload(ctx)
		case "T":
			b.month = time.Now().UTC().Format("2006-01")
			b.reload(ctx)
		case "r":
			b.reload(ctx)
		}
	}
	return b, nil
}

func (b *Budget) View(width, height int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "  Budget for %s\n\n", b.month)

	sb.WriteString(styles.HeaderText.Render("Buckets"))
	sb.WriteString(styles.HeaderRule.Render(" " + strings.Repeat(styles.RuleChar, maxInt(width-7, 4))))
	sb.WriteString("\n")

	nameW := maxInt(width-2-2-11-2-11-2-11, 8)
	sb.WriteString("  ")
	sb.WriteString(styles.FooterKey.Render(padRight(truncate("NAME", nameW), nameW)))
	sb.WriteString("  ")
	sb.WriteString(styles.FooterKey.Render(padLeft("ALLOC", 11)))
	sb.WriteString("  ")
	sb.WriteString(styles.FooterKey.Render(padLeft("SPENT", 11)))
	sb.WriteString("  ")
	sb.WriteString(styles.FooterKey.Render(padLeft("LEFT", 11)))
	sb.WriteString("\n")

	if len(b.spends) == 0 {
		sb.WriteString(styles.UnknownCategory.Render("  (no buckets)"))
		sb.WriteString("\n")
	} else {
		for _, s := range b.spends {
			sb.WriteString(b.renderBucket(s, nameW))
			sb.WriteString("\n")
		}
	}

	if len(b.unassigned) > 0 {
		sb.WriteString("\n")
		sb.WriteString(styles.HeaderText.Render("Unassigned"))
		sb.WriteString(styles.HeaderRule.Render(" " + strings.Repeat(styles.RuleChar, maxInt(width-11, 4))))
		sb.WriteString("\n")
		for _, s := range b.unassigned {
			sb.WriteString(b.renderUnassigned(s))
			sb.WriteString("\n")
		}
	}
	if len(b.spends) == 0 && len(b.unassigned) == 0 {
		sb.WriteString(styles.UnknownCategory.Render("  (nothing to show for this month)"))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (b *Budget) renderBucket(s ports.BucketSpend, nameW int) string {
	name := padRight(truncate(s.BucketName, nameW), nameW)
	remaining := s.AllocatedMinor - s.SpentMinor
	allocStr := formatMinor(s.AllocatedMinor, s.Currency)
	spentStr := formatMinor(s.SpentMinor, s.Currency)
	leftStr := formatMinor(remaining, s.Currency)
	leftStyle := lipgloss.NewStyle().Foreground(styles.Dim)
	if remaining < 0 {
		leftStyle = styles.AmountOut
	} else if remaining > 0 {
		leftStyle = styles.AmountIn
	}
	alloc := styles.FooterKey.Render(padLeft(allocStr, 11))
	spent := styles.FooterKey.Render(padLeft(spentStr, 11))
	left := leftStyle.Render(padLeft(leftStr, 11))
	return fmt.Sprintf("  %s  %s  %s  %s", name, alloc, spent, left)
}

func (b *Budget) renderUnassigned(s ports.BucketSpend) string {
	amountStr := formatMinor(s.SpentMinor, s.Currency)
	countStr := fmt.Sprintf("%d tx", s.Count)
	row := styles.AmountOut.Render(padLeft(amountStr, 14)) + "  " + styles.FooterKey.Render(countStr)
	return "  " + row
}

func padLeft(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return strings.Repeat(" ", n-len(s)) + s
}

func (b *Budget) Hints(width int) hints.FooterHints {
	return hints.FooterHints{
		Mode: "Normal",
		Keys: []hints.KeyHint{
			{Key: "Tab", Label: "focus"},
			{Key: "a", Label: "archive"},
			{Key: "n", Label: "new"},
			{Key: "?", Label: "help"},
		},
	}
}

func formatMinor(minor int64, currency string) string {
	cur := valueobjects.Currency(currency)
	m, err := valueobjects.New(minor, cur)
	if err != nil {
		return fmt.Sprintf("%d %s", minor, currency)
	}
	return m.DecimalString() + " " + currency
}

type Recipes struct {
	deps       Deps
	rows       []recipeRow
	cursor     int
	active     string
	statusMsg  string
}

type recipeRow struct {
	name        string
	description string
	net         bool
}

func NewRecipes() *Recipes { return &Recipes{} }

func (r *Recipes) Title() string { return "Recipes" }

func (r *Recipes) Init(ctx context.Context, deps Deps) tea.Cmd {
	r.deps = deps
	r.reload(ctx)
	return nil
}

func (r *Recipes) reload(ctx context.Context) {
	if r.deps.RecipeSvc == nil {
		r.statusMsg = "recipe service not available"
		return
	}
	all, err := r.deps.RecipeSvc.LoadAll(ctx)
	if err != nil {
		r.statusMsg = "load: " + err.Error()
		return
	}
	active, _ := r.deps.RecipeSvc.GetActiveName(ctx)
	r.active = active
	r.rows = r.rows[:0]
	for _, rec := range all {
		r.rows = append(r.rows, recipeRow{
			name:        rec.Name,
			description: rec.Description,
			net:         rec.Net,
		})
	}
	if r.cursor >= len(r.rows) {
		r.cursor = len(r.rows) - 1
	}
	if r.cursor < 0 {
		r.cursor = 0
	}
	r.statusMsg = fmt.Sprintf("%d recipes · active: %s", len(r.rows), r.active)
}

func (r *Recipes) Update(msg tea.Msg) (Screen, tea.Cmd) {
	ctx := context.Background()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if r.cursor < len(r.rows)-1 {
				r.cursor++
			}
		case "k", "up":
			if r.cursor > 0 {
				r.cursor--
			}
		case "u":
			if r.cursor < len(r.rows) && r.deps.RecipeSvc != nil {
				if err := r.deps.RecipeSvc.SetActiveName(ctx, r.rows[r.cursor].name); err != nil {
					r.statusMsg = "use: " + err.Error()
				} else {
					r.active = r.rows[r.cursor].name
					r.statusMsg = "active recipe → " + r.active
				}
			}
		}
	}
	return r, nil
}

func (r *Recipes) View(width, height int) string {
	if len(r.rows) == 0 {
		empty := styles.UnknownCategory.Render("  (no recipes — drop a .toml in $LEDGER_RECIPES_DIR)")
		return fmt.Sprintf("%s\n  active: %s\n", empty, r.active)
	}
	var b strings.Builder
	nameW := 24
	netW := 6
	descW := width - (1 + 1 + nameW + 2 + netW + 2)
	if descW < 4 {
		descW = 4
	}
	b.WriteString(styles.HeaderText.Render(fmt.Sprintf(" %s %s %s %s",
		padBoth("NAME", nameW),
		padBoth("NET", netW),
		"ACTIVE",
		"DESCRIPTION",
	)))
	b.WriteString("\n")
	b.WriteString(styles.HeaderRule.Render(strings.Repeat(styles.RuleChar, width)))
	b.WriteString("\n")

	visible := r.rows
	if height > 2 && len(visible) > height-2 {
		visible = visible[:height-2]
	}
	for i, row := range visible {
		b.WriteString(r.renderRow(i, row, width))
		b.WriteString("\n")
	}
	return b.String()
}

func (r *Recipes) renderRow(i int, row recipeRow, width int) string {
	isCursor := i == r.cursor
	isActive := row.name == r.active
	rowStyle := lipgloss.Style{}
	if isCursor {
		rowStyle = styles.CursorRow
	}
	cursorChar := " "
	if isCursor {
		cursorChar = styles.CursorGlyph
	}
	nameW := 24
	netW := 6
	descW := width - (1 + 1 + nameW + 2 + netW + 2)
	if descW < 4 {
		descW = 4
	}
	net := "no"
	if row.net {
		net = "yes"
	}
	netStyled := styles.FooterKey.Render(padRight(net, netW))
	activeMark := "  "
	if isActive {
		activeMark = lipgloss.NewStyle().Foreground(styles.Accent).Bold(true).Render("✓")
	}
	name := padRight(truncate(row.name, nameW), nameW)
	desc := truncate(row.description, descW)
	body := fmt.Sprintf("%s %s %s %s %s %s",
		cursorChar, name, netStyled, activeMark, "", desc,
	)
	return rowStyle.Render(truncateToWidth(body, width))
}

func (r *Recipes) Hints(width int) hints.FooterHints {
	return hints.FooterHints{
		Mode: "Normal",
		Keys: []hints.KeyHint{
			{Key: "j/k", Label: "nav"},
			{Key: "u", Label: "use"},
			{Key: "n", Label: "new"},
			{Key: "e", Label: "edit"},
			{Key: "?", Label: "help"},
		},
	}
}

func linkerGroupRepo(d Deps) ports.GroupRepository { return d.GroupRepo }

func annSvcFromDeps(d Deps) *services.AnnotationService {
	return services.NewAnnotationService(services.AnnotationDeps{
		DB:         d.DB,
		TxRepo:     d.TxRepo,
		TagRepo:    d.TagRepo,
		BucketRepo: d.BucketRepo,
		AuditRepo:  d.AuditRepo,
		BatchRepo:  d.BatchRepo,
		OverlaySvc: d.OverlaySvc,
	})
}
