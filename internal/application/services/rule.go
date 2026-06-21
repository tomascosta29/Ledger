package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
)

type RuleDeps struct {
	TxRepo      ports.TransactionRepository
	TagRepo     ports.TagRepository
	BucketRepo  ports.BucketRepository
	CategoryRepo ports.CategoryRepository
	RuleRepo    ports.RuleRepository
	AnnService  *AnnotationService
	Now         func() time.Time
}

type RuleService struct {
	deps RuleDeps
}

type RuleMatch struct {
	RuleID    int64
	RuleName  string
	TxID      int64
	Applied   []string
}

type RuleApplyResult struct {
	Matched  int
	Applied  int
	Skipped  int
	ByRule   map[int64]int
	Errors   []error
}

func NewRuleService(d RuleDeps) *RuleService {
	return &RuleService{deps: d}
}

func (s *RuleService) Apply(ctx context.Context) (*RuleApplyResult, error) {
	rules, err := s.deps.RuleRepo.List(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	if len(rules) == 0 {
		return &RuleApplyResult{ByRule: map[int64]int{}}, nil
	}
	txs, err := s.deps.TxRepo.FindAll(ctx, ports.TxFindOptions{Limit: 100000})
	if err != nil {
		return nil, fmt.Errorf("list transactions: %w", err)
	}
	result := &RuleApplyResult{ByRule: map[int64]int{}}
	for _, tx := range txs {
		for _, rule := range rules {
			if !s.matches(tx, rule) {
				continue
			}
			result.Matched++
			applied, err := s.applyOne(ctx, tx, rule)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("rule %q on txn %d: %w", rule.Name, tx.ID, err))
				continue
			}
			if applied {
				result.Applied++
				result.ByRule[rule.ID]++
			} else {
				result.Skipped++
			}
		}
	}
	return result, nil
}

func (s *RuleService) matches(tx *entities.Transaction, rule *entities.Rule) bool {
	if rule.MatchPartner != nil {
		partner := ""
		if tx.PartnerName != nil {
			partner = *tx.PartnerName
		}
		if !strings.EqualFold(partner, *rule.MatchPartner) {
			return false
		}
	}
	if rule.MatchDescription != nil {
		if !strings.Contains(strings.ToLower(tx.Description), strings.ToLower(*rule.MatchDescription)) {
			return false
		}
	}
	if !rule.MatchesAmount(tx.Amount.Amount) {
		return false
	}
	return true
}

func (s *RuleService) applyOne(ctx context.Context, tx *entities.Transaction, rule *entities.Rule) (bool, error) {
	did := false
	if rule.SetCategory != nil && tx.CategoryID == nil {
		if err := s.deps.AnnService.Categorize(ctx, tx.ID, *rule.SetCategory, nil); err != nil {
			return false, err
		}
		// Re-read the FK the service resolved, so the in-memory tx is in
		// sync for downstream checks (e.g. bucket on same rule pass).
		if c, err := s.deps.CategoryRepo.GetByName(ctx, *rule.SetCategory); err == nil {
			tx.CategoryID = &c.ID
		}
		did = true
	}
	if rule.SetBucketID != nil && tx.BucketID == nil {
		bucketName := s.bucketName(ctx, *rule.SetBucketID)
		if bucketName == "" {
			return did, nil
		}
		var currentCategory string
		if tx.CategoryID != nil {
			if c, err := s.deps.CategoryRepo.GetByID(ctx, *tx.CategoryID); err == nil {
				currentCategory = c.Name
			}
		}
		if err := s.deps.AnnService.Categorize(ctx, tx.ID, currentCategory, &bucketName); err != nil {
			return false, err
		}
		did = true
	}
	if len(rule.AddTags) > 0 {
		existing, _ := s.deps.TagRepo.ListByTransaction(ctx, tx.ID)
		toAdd := []string{}
		for _, t := range rule.AddTags {
			if !contains(existing, t) {
				toAdd = append(toAdd, t)
			}
		}
		if len(toAdd) > 0 {
			if err := s.deps.AnnService.BulkAddTags(ctx, []int64{tx.ID}, toAdd); err != nil {
				return false, err
			}
			did = true
		}
	}
	return did, nil
}

func (s *RuleService) bucketName(ctx context.Context, id int64) string {
	if s.deps.BucketRepo == nil {
		return ""
	}
	bucket, err := s.deps.BucketRepo.GetByID(ctx, id)
	if err != nil || bucket == nil {
		return ""
	}
	return bucket.Name
}
