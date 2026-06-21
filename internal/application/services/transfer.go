package services

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
	"github.com/tomascosta29/Ledger/internal/domain/valueobjects"
)

type TransferDetectionDeps struct {
	TxRepo    ports.TransactionRepository
	GroupRepo ports.GroupRepository
	AuditRepo ports.AuditLogRepository
	OverlaySvc ports.OverlayService
	Now       func() time.Time
	WindowDays int
}

type TransferService struct {
	deps TransferDetectionDeps
}

func NewTransferService(d TransferDetectionDeps) *TransferService {
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	if d.WindowDays == 0 {
		d.WindowDays = 3
	}
	return &TransferService{deps: d}
}

type TransferCandidate struct {
	OutID    int64
	OutDate  string
	OutPartner string
	OutAmount int64
	InID     int64
	InDate   string
	InPartner string
	InAmount int64
	Currency valueobjects.Currency
	Score    int
}

func (s *TransferService) Detect(ctx context.Context) ([]TransferCandidate, error) {
	txs, err := s.deps.TxRepo.FindAll(ctx, ports.TxFindOptions{Limit: 100000})
	if err != nil {
		return nil, err
	}
	sort.Slice(txs, func(i, j int) bool {
		return txs[i].EffectiveDate < txs[j].EffectiveDate
	})

	var positives, negatives []*entities.Transaction
	for _, t := range txs {
		if t.Amount.IsPositive() {
			positives = append(positives, t)
		} else if t.Amount.IsNegative() {
			negatives = append(negatives, t)
		}
	}

	used := make(map[int64]bool)
	var out []TransferCandidate
	for _, out_tx := range negatives {
		if used[out_tx.ID] {
			continue
		}
		for _, in_tx := range positives {
			if used[in_tx.ID] {
				continue
			}
			if out_tx.Amount.Currency != in_tx.Amount.Currency {
				continue
			}
			if out_tx.Amount.Amount != -in_tx.Amount.Amount {
				continue
			}
			if !s.withinWindow(out_tx.EffectiveDate, in_tx.EffectiveDate) {
				continue
			}
			score := s.score(out_tx, in_tx)
			out = append(out, TransferCandidate{
				OutID:      out_tx.ID,
				OutDate:    out_tx.EffectiveDate,
				OutPartner: derefStr(out_tx.PartnerName),
				OutAmount:  out_tx.Amount.Amount,
				InID:       in_tx.ID,
				InDate:     in_tx.EffectiveDate,
				InPartner:  derefStr(in_tx.PartnerName),
				InAmount:   in_tx.Amount.Amount,
				Currency:   out_tx.Amount.Currency,
				Score:      score,
			})
			used[out_tx.ID] = true
			used[in_tx.ID] = true
			break
		}
	}
	return out, nil
}

func (s *TransferService) withinWindow(a, b string) bool {
	ta, errA := time.Parse("2006-01-02", a)
	tb, errB := time.Parse("2006-01-02", b)
	if errA != nil || errB != nil {
		return false
	}
	delta := ta.Sub(tb)
	if delta < 0 {
		delta = -delta
	}
	return delta <= time.Duration(s.deps.WindowDays)*24*time.Hour
}

func (s *TransferService) score(out, in *entities.Transaction) int {
	score := 1
	if out.EffectiveDate == in.EffectiveDate {
		score += 2
	}
	if derefStr(out.PartnerName) != "" && derefStr(in.PartnerName) != "" &&
		strings.EqualFold(derefStr(out.PartnerName), derefStr(in.PartnerName)) {
		score += 1
	}
	if out.Description != "" && in.Description != "" {
		if strings.Contains(strings.ToLower(out.Description), strings.ToLower(in.Description)) ||
			strings.Contains(strings.ToLower(in.Description), strings.ToLower(out.Description)) {
			score += 1
		}
	}
	return score
}

func (s *TransferService) Confirm(ctx context.Context, c TransferCandidate) (int64, error) {
	groupID, err := s.deps.GroupRepo.CreateGroup(ctx, &entities.TransactionGroup{})
	if err != nil {
		return 0, err
	}
	if err := s.deps.GroupRepo.AddMember(ctx, groupID, c.OutID, "from"); err != nil {
		return 0, err
	}
	if err := s.deps.GroupRepo.AddMember(ctx, groupID, c.InID, "to"); err != nil {
		return 0, err
	}
	now := s.deps.Now()
	entries := []*entities.AuditEntry{
		{TableName: "transactions", RecordID: c.OutID, Action: entities.AuditActionTransferLink,
			Field: linkStr("group_id"), NewValue: i64Str(groupID), CreatedAt: now},
		{TableName: "transactions", RecordID: c.InID, Action: entities.AuditActionTransferLink,
			Field: linkStr("group_id"), NewValue: i64Str(groupID), CreatedAt: now},
	}
	if err := s.deps.AuditRepo.AppendBatch(ctx, entries); err != nil {
		return 0, fmt.Errorf("append audit: %w", err)
	}
	if s.deps.OverlaySvc != nil {
		if err := s.deps.OverlaySvc.Rebuild(ctx); err != nil {
			return 0, fmt.Errorf("rebuild overlay: %w", err)
		}
	}
	return groupID, nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
