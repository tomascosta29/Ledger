package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
)

type SummaryDeps struct {
	OverlayRepo ports.OverlayRepository
	RecipeRepo  ports.RecipeRepository
	TagRepo     ports.TagRepository
	Now         func() time.Time
}

type SummaryService struct {
	deps SummaryDeps
}

type SummaryLine struct {
	Currency valueobjects.Currency
	Income   int64
	Expense  int64
	Net      int64
	Count    int
}

type SummaryResult struct {
	RecipeName string
	Month      string
	Lines      []SummaryLine
}

func NewSummaryService(d SummaryDeps) *SummaryService {
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	return &SummaryService{deps: d}
}

func (s *SummaryService) Run(ctx context.Context, recipeName, month string) (*SummaryResult, error) {
	if month == "" {
		month = s.deps.Now().Format("2006-01")
	}
	if recipeName == "" {
		var err error
		recipeName, err = s.deps.RecipeRepo.GetActiveName(ctx)
		if err != nil {
			return nil, fmt.Errorf("get active recipe: %w", err)
		}
		if recipeName == "" {
			return nil, fmt.Errorf("no recipe specified and no active recipe set; use `ledger recipe use <name>`")
		}
	}
	recipe, err := s.deps.RecipeRepo.LoadByName(ctx, recipeName)
	if err != nil {
		return nil, fmt.Errorf("load recipe %q: %w", recipeName, err)
	}

	start := month + "-01"
	t, _ := time.Parse("2006-01-02", start)
	end := t.AddDate(0, 1, -1).Format("2006-01-02")

	opts := ports.OverlayFindOptions{
		Filters: ports.OverlayFilters{
			SourceKinds: []ports.SourceKind{ports.SourceRaw, ports.SourceSplitHeader, ports.SourceSplitChild, ports.SourceGroup},
			StartDate:   &start,
			EndDate:     &end,
		},
		Sort:  ports.OverlaySortByDate,
		Order: ports.SortAsc,
	}
	rows, err := s.deps.OverlayRepo.FindAll(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("query overlay: %w", err)
	}

	byCurrency := make(map[valueobjects.Currency]*SummaryLine)
	for _, r := range rows {
		if !s.matches(r, recipe.Include, true) {
			continue
		}
		if s.matches(r, recipe.Exclude, false) {
			continue
		}
		cur := r.Amount.Currency
		line, ok := byCurrency[cur]
		if !ok {
			line = &SummaryLine{Currency: cur}
			byCurrency[cur] = line
		}
		if r.Amount.IsPositive() {
			line.Income += r.Amount.Amount
		} else if r.Amount.IsNegative() {
			line.Expense += r.Amount.Amount
		}
		line.Count++
	}

	out := &SummaryResult{
		RecipeName: recipeName,
		Month:      month,
	}
	for _, line := range byCurrency {
		line.Net = line.Income + line.Expense
		out.Lines = append(out.Lines, *line)
	}
	return out, nil
}

func (s *SummaryService) matches(r *ports.OverlayTransaction, clauses []entities.Clause, include bool) bool {
	if len(clauses) == 0 {
		return include
	}
	for _, c := range clauses {
		if s.matchClause(r, c) {
			return true
		}
	}
	return false
}

func (s *SummaryService) matchClause(r *ports.OverlayTransaction, c entities.Clause) bool {
	field := strings.ToLower(c.Field)
	op := strings.ToLower(c.Op)
	switch field {
	case "category":
		return matchString(r.Category, op, c.Value)
	case "partner":
		partner := ""
		if r.PartnerName != nil {
			partner = *r.PartnerName
		}
		return matchString(partner, op, c.Value)
	case "bucket":
		bucket := fmt.Sprintf("%v", r.BucketID)
		return matchString(bucket, op, c.Value)
	case "tag":
		tags := strings.Split(r.Tags, ",")
		for _, t := range tags {
			if matchString(strings.TrimSpace(t), op, c.Value) {
				return true
			}
		}
		return false
	case "source_kind":
		return matchString(string(r.SourceKind), op, c.Value)
	}
	return false
}

func matchString(have, op, want string) bool {
	switch op {
	case "is", "eq", "==":
		return have == want
	case "not", "ne", "!=":
		return have != want
	case "contains", "has":
		return strings.Contains(strings.ToLower(have), strings.ToLower(want))
	}
	return false
}
