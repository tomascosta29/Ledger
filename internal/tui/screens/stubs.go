package screens

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/application/services"
	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
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

func (c *Categorizer) View() string {
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
		return fmt.Sprintf("  %s: %s_\n", label, c.input)
	}
	if len(c.rows) == 0 {
		return "  (no Unknown transactions — try Manager or Categorizer with filter)\n"
	}
	var b strings.Builder
	b.WriteString("  ID    DATE         AMOUNT       DESCRIPTION    (keys: c cat · b bucket · t tag · j/k nav)\n")
	for i, r := range c.rows {
		marker := "  "
		if i == c.cursor {
			marker = "> "
		}
		fmt.Fprintf(&b, "%s%-5d  %-11s  %-11s  %s\n",
			marker, r.id, r.date, r.amount, r.desc)
	}
	return b.String()
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

func (l *Linker) View() string {
	var sb strings.Builder
	sb.WriteString("  Candidates & groups (j/k · enter: link candidate):\n")
	if len(l.cands) == 0 {
		sb.WriteString("    (no candidates)\n")
	}
	for i, c := range l.cands {
		marker := "    "
		if l.cursor == i {
			marker = ">   "
		}
		fmt.Fprintf(&sb, "%sscore %d: out=%s | in=%s\n", marker, c.score, c.outTxt, c.inTxt)
	}
	if len(l.groups) > 0 {
		sb.WriteString("\n  Existing groups:\n")
		for i, g := range l.groups {
			marker := "    "
			if l.cursor == len(l.cands)+i {
				marker = ">   "
			}
			fmt.Fprintf(&sb, "%s#%d  %s\n", marker, g.id, g.note)
		}
	}
	return sb.String()
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

func (b *Budget) View() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "  Budget for %s    (n/p: ±month · T: today · r: reload)\n\n", b.month)
	fmt.Fprintf(&sb, "  %-22s  %12s  %12s  %12s  %s\n", "BUCKET", "ALLOCATED", "SPENT", "REMAINING", "TX")
	for _, s := range b.spends {
		remaining := s.AllocatedMinor - s.SpentMinor
		fmt.Fprintf(&sb, "  %-22s  %12s  %12s  %12s  %d\n",
			s.BucketName,
			formatMinor(s.AllocatedMinor, s.Currency),
			formatMinor(s.SpentMinor, s.Currency),
			formatMinor(remaining, s.Currency),
			s.Count,
		)
	}
	if len(b.unassigned) > 0 {
		sb.WriteString("\n  Unassigned:\n")
		for _, s := range b.unassigned {
			fmt.Fprintf(&sb, "    %-20s  %12s  %d tx\n",
				s.Currency,
				formatMinor(s.SpentMinor, s.Currency),
				s.Count,
			)
		}
	}
	if len(b.spends) == 0 && len(b.unassigned) == 0 {
		sb.WriteString("\n  (nothing to show for this month)\n")
	}
	return sb.String()
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

func (r *Recipes) View() string {
	if len(r.rows) == 0 {
		return fmt.Sprintf("  (no recipes — drop a .toml in $LEDGER_RECIPES_DIR)\n  active: %s\n", r.active)
	}
	var b strings.Builder
	b.WriteString("  NAME                            NET     DESCRIPTION\n")
	for i, row := range r.rows {
		marker := "  "
		if row.name == r.active {
			marker = "* "
		}
		if i == r.cursor {
			marker = "> "
		}
		net := "no"
		if row.net {
			net = "yes"
		}
		fmt.Fprintf(&b, "%s%-30s  %-6s  %s\n", marker, row.name, net, row.description)
	}
	return b.String()
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
