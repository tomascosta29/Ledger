package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tomascosta29/Ledger/internal/application/ports"
	"github.com/tomascosta29/Ledger/internal/domain/entities"
)

type AnnotationDeps struct {
	DB           *sql.DB
	TxRepo       ports.TransactionRepository
	TagRepo      ports.TagRepository
	BucketRepo   ports.BucketRepository
	CategoryRepo ports.CategoryRepository
	AuditRepo    ports.AuditLogRepository
	BatchRepo    ports.ImportBatchRepository
	OverlaySvc   ports.OverlayService
	Now          func() time.Time
}

type AnnotationService struct {
	deps AnnotationDeps
}

func NewAnnotationService(d AnnotationDeps) *AnnotationService {
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	return &AnnotationService{deps: d}
}

func (s *AnnotationService) Categorize(ctx context.Context, txID int64, category string, bucketName *string) error {
	if category == "" {
		return errors.New("category is empty")
	}
	old, err := s.deps.TxRepo.GetByID(ctx, txID)
	if err != nil {
		return fmt.Errorf("load transaction: %w", err)
	}
	newCatID, err := s.resolveCategoryID(ctx, category)
	if err != nil {
		return err
	}
	bucket, err := s.resolveBucket(ctx, bucketName, string(old.Amount.Currency))
	if err != nil {
		return err
	}

	oldName := s.categoryNameOrEmpty(ctx, old.CategoryID)

	return s.runTx(ctx, func(tx *sql.Tx) error {
		now := s.deps.Now()
		if !sameCategoryID(old.CategoryID, newCatID) {
			if err := s.deps.TxRepo.SetCategoryDBTX(ctx, tx, txID, newCatID); err != nil {
				return err
			}
			if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, &entities.AuditEntry{
				TableName: "transactions",
				RecordID:  txID,
				Action:    entities.AuditActionCategorize,
				Field:     strPtr("category"),
				OldValue:  strPtr(oldName),
				NewValue:  strPtr(category),
				CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		if bucket != nil && !sameBucketID(old.BucketID, &bucket.ID) {
			if err := s.deps.TxRepo.SetBucketDBTX(ctx, tx, txID, &bucket.ID); err != nil {
				return err
			}
			if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, &entities.AuditEntry{
				TableName: "transactions",
				RecordID:  txID,
				Action:    entities.AuditActionBucket,
				Field:     strPtr("bucket_id"),
				OldValue:  bucketIDToStringPtr(old.BucketID),
				NewValue:  bucketIDToStringPtr(&bucket.ID),
				CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		if sameCategoryID(old.CategoryID, newCatID) && (bucket == nil || sameBucketID(old.BucketID, &bucket.ID)) {
			return nil
		}
		return s.deps.OverlaySvc.RebuildWithTx(ctx, tx)
	})
}

// CategorizeViaRule mirrors Categorize but writes RuleApply audit rows
// instead of Categorize/Bucket. Used by RuleService.Apply when --overwrite
// is set, so the audit log distinguishes operator-driven annotations
// from rule-driven bulk fixes. The caller is responsible for enforcing
// any "no overwrite" gate; this method always writes the change.
func (s *AnnotationService) CategorizeViaRule(ctx context.Context, txID int64, category string, bucketName *string) error {
	if category == "" {
		return errors.New("category is empty")
	}
	old, err := s.deps.TxRepo.GetByID(ctx, txID)
	if err != nil {
		return fmt.Errorf("load transaction: %w", err)
	}
	newCatID, err := s.resolveCategoryID(ctx, category)
	if err != nil {
		return err
	}
	bucket, err := s.resolveBucket(ctx, bucketName, string(old.Amount.Currency))
	if err != nil {
		return err
	}

	oldName := s.categoryNameOrEmpty(ctx, old.CategoryID)

	return s.runTx(ctx, func(tx *sql.Tx) error {
		now := s.deps.Now()
		if !sameCategoryID(old.CategoryID, newCatID) {
			if err := s.deps.TxRepo.SetCategoryDBTX(ctx, tx, txID, newCatID); err != nil {
				return err
			}
			if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, &entities.AuditEntry{
				TableName: "transactions",
				RecordID:  txID,
				Action:    entities.AuditActionRuleApply,
				Field:     strPtr("category"),
				OldValue:  strPtr(oldName),
				NewValue:  strPtr(category),
				CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		if bucket != nil && !sameBucketID(old.BucketID, &bucket.ID) {
			if err := s.deps.TxRepo.SetBucketDBTX(ctx, tx, txID, &bucket.ID); err != nil {
				return err
			}
			if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, &entities.AuditEntry{
				TableName: "transactions",
				RecordID:  txID,
				Action:    entities.AuditActionRuleApply,
				Field:     strPtr("bucket_id"),
				OldValue:  bucketIDToStringPtr(old.BucketID),
				NewValue:  bucketIDToStringPtr(&bucket.ID),
				CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		if sameCategoryID(old.CategoryID, newCatID) && (bucket == nil || sameBucketID(old.BucketID, &bucket.ID)) {
			return nil
		}
		return s.deps.OverlaySvc.RebuildWithTx(ctx, tx)
	})
}

func (s *AnnotationService) SetBucket(ctx context.Context, txID int64, bucketName string) error {
	old, err := s.deps.TxRepo.GetByID(ctx, txID)
	if err != nil {
		return fmt.Errorf("load transaction: %w", err)
	}
	bucket, err := s.resolveBucket(ctx, &bucketName, string(old.Amount.Currency))
	if err != nil {
		return err
	}
	if sameBucketID(old.BucketID, &bucket.ID) {
		return nil
	}

	oldBucket := bucketIDToStringPtr(old.BucketID)
	newBucket := bucketIDToStringPtr(&bucket.ID)
	return s.runTx(ctx, func(tx *sql.Tx) error {
		if err := s.deps.TxRepo.SetBucketDBTX(ctx, tx, txID, &bucket.ID); err != nil {
			return err
		}
		entry := &entities.AuditEntry{
			TableName: "transactions",
			RecordID:  txID,
			Action:    entities.AuditActionBucket,
			Field:     strPtr("bucket_id"),
			OldValue:  oldBucket,
			NewValue:  newBucket,
			CreatedAt: s.deps.Now(),
		}
		if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, entry); err != nil {
			return err
		}
		return s.deps.OverlaySvc.RebuildWithTx(ctx, tx)
	})
}

func (s *AnnotationService) BulkSetBucket(ctx context.Context, txIDs []int64, bucketName string) error {
	if len(txIDs) == 0 {
		return errors.New("no transaction ids provided")
	}
	uniqueIDs := dedupIDs(txIDs)
	for _, id := range uniqueIDs {
		if err := s.SetBucket(ctx, id, bucketName); err != nil {
			return fmt.Errorf("txn %d: %w", id, err)
		}
	}
	return nil
}

func (s *AnnotationService) SetHidden(ctx context.Context, txID int64, hidden bool) error {
	old, err := s.deps.TxRepo.GetByID(ctx, txID)
	if err != nil {
		return fmt.Errorf("load transaction: %w", err)
	}
	if old.IsHidden == hidden {
		return nil
	}

	return s.runTx(ctx, func(tx *sql.Tx) error {
		if err := s.deps.TxRepo.SetHiddenDBTX(ctx, tx, txID, hidden); err != nil {
			return err
		}
		entry := &entities.AuditEntry{
			TableName: "transactions",
			RecordID:  txID,
			Action:    entities.AuditActionVisibility,
			Field:     strPtr("is_hidden"),
			OldValue:  boolStrPtr(old.IsHidden),
			NewValue:  boolStrPtr(hidden),
			CreatedAt: s.deps.Now(),
		}
		if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, entry); err != nil {
			return err
		}
		return s.deps.OverlaySvc.RebuildWithTx(ctx, tx)
	})
}

func (s *AnnotationService) AddTag(ctx context.Context, txID int64, tag string) error {
	existing, err := s.deps.TagRepo.ListByTransaction(ctx, txID)
	if err != nil {
		return fmt.Errorf("load tags: %w", err)
	}
	if contains(existing, tag) {
		return nil
	}

	return s.runTx(ctx, func(tx *sql.Tx) error {
		if err := s.deps.TagRepo.AddDBTX(ctx, tx, txID, tag); err != nil {
			return err
		}
		newTags := append(existing, tag)
		entry := &entities.AuditEntry{
			TableName: "transactions",
			RecordID:  txID,
			Action:    entities.AuditActionTag,
			Field:     strPtr("tags"),
			OldValue:  strPtr(joinTags(existing)),
			NewValue:  strPtr(joinTags(newTags)),
			CreatedAt: s.deps.Now(),
		}
		if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, entry); err != nil {
			return err
		}
		return s.deps.OverlaySvc.RebuildWithTx(ctx, tx)
	})
}

func (s *AnnotationService) RemoveTag(ctx context.Context, txID int64, tag string) error {
	existing, err := s.deps.TagRepo.ListByTransaction(ctx, txID)
	if err != nil {
		return fmt.Errorf("load tags: %w", err)
	}
	if !contains(existing, tag) {
		return nil
	}

	return s.runTx(ctx, func(tx *sql.Tx) error {
		if err := s.deps.TagRepo.RemoveDBTX(ctx, tx, txID, tag); err != nil {
			return err
		}
		remaining := removeFrom(existing, tag)
		entry := &entities.AuditEntry{
			TableName: "transactions",
			RecordID:  txID,
			Action:    entities.AuditActionTag,
			Field:     strPtr("tags"),
			OldValue:  strPtr(joinTags(existing)),
			NewValue:  strPtr(joinTags(remaining)),
			CreatedAt: s.deps.Now(),
		}
		if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, entry); err != nil {
			return err
		}
		return s.deps.OverlaySvc.RebuildWithTx(ctx, tx)
	})
}

func (s *AnnotationService) runTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.deps.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func strPtr(s string) *string { return &s }

func boolStrPtr(b bool) *string {
	if b {
		s := "true"
		return &s
	}
	s := "false"
	return &s
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func removeFrom(s []string, v string) []string {
	out := make([]string, 0, len(s))
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

func joinTags(s []string) string {
	if len(s) == 0 {
		return ""
	}
	out := s[0]
	for _, x := range s[1:] {
		out += "," + x
	}
	return out
}

func (s *AnnotationService) BulkCategorize(ctx context.Context, txIDs []int64, category string, bucketName *string) error {
	if category == "" {
		return errors.New("category is empty")
	}
	if len(txIDs) == 0 {
		return errors.New("no transaction ids provided")
	}
	uniqueIDs := dedupIDs(txIDs)

	oldByID := make(map[int64]*entities.Transaction, len(uniqueIDs))
	for _, id := range uniqueIDs {
		txn, err := s.deps.TxRepo.GetByID(ctx, id)
		if err != nil {
			return fmt.Errorf("load transaction %d: %w", id, err)
		}
		oldByID[id] = txn
	}

	var bucket *entities.Bucket
	if bucketName != nil {
		currencies := make(map[string]struct{}, len(uniqueIDs))
		for _, id := range uniqueIDs {
			currencies[string(oldByID[id].Amount.Currency)] = struct{}{}
		}
		if len(currencies) > 1 {
			return fmt.Errorf("--bucket cannot span multiple currencies: %d distinct currencies in selection", len(currencies))
		}
		var cur string
		for c := range currencies {
			cur = c
		}
		b, err := s.resolveBucket(ctx, bucketName, cur)
		if err != nil {
			return err
		}
		bucket = b
	}
	newCatID, err := s.resolveCategoryID(ctx, category)
	if err != nil {
		return err
	}

	return s.runTx(ctx, func(tx *sql.Tx) error {
		wrote := false
		now := s.deps.Now()
		for _, id := range uniqueIDs {
			old := oldByID[id]
			if !sameCategoryID(old.CategoryID, newCatID) {
				oldName := s.categoryNameOrEmpty(ctx, old.CategoryID)
				if err := s.deps.TxRepo.SetCategoryDBTX(ctx, tx, id, newCatID); err != nil {
					return err
				}
				if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, &entities.AuditEntry{
					TableName: "transactions",
					RecordID:  id,
					Action:    entities.AuditActionCategorize,
					Field:     strPtr("category"),
					OldValue:  strPtr(oldName),
					NewValue:  strPtr(category),
					CreatedAt: now,
				}); err != nil {
					return err
				}
				wrote = true
			}
			if bucket != nil && !sameBucketID(old.BucketID, &bucket.ID) {
				if err := s.deps.TxRepo.SetBucketDBTX(ctx, tx, id, &bucket.ID); err != nil {
					return err
				}
				if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, &entities.AuditEntry{
					TableName: "transactions",
					RecordID:  id,
					Action:    entities.AuditActionBucket,
					Field:     strPtr("bucket_id"),
					OldValue:  bucketIDToStringPtr(old.BucketID),
					NewValue:  bucketIDToStringPtr(&bucket.ID),
					CreatedAt: now,
				}); err != nil {
					return err
				}
				wrote = true
			}
		}
		if !wrote {
			return nil
		}
		return s.deps.OverlaySvc.RebuildWithTx(ctx, tx)
	})
}

func (s *AnnotationService) BulkSetHidden(ctx context.Context, txIDs []int64, hidden bool) error {
	if len(txIDs) == 0 {
		return errors.New("no transaction ids provided")
	}
	uniqueIDs := dedupIDs(txIDs)

	oldByID := make(map[int64]bool, len(uniqueIDs))
	for _, id := range uniqueIDs {
		txn, err := s.deps.TxRepo.GetByID(ctx, id)
		if err != nil {
			return fmt.Errorf("load transaction %d: %w", id, err)
		}
		oldByID[id] = txn.IsHidden
	}

	return s.runTx(ctx, func(tx *sql.Tx) error {
		wrote := false
		now := s.deps.Now()
		for _, id := range uniqueIDs {
			if oldByID[id] == hidden {
				continue
			}
			if err := s.deps.TxRepo.SetHiddenDBTX(ctx, tx, id, hidden); err != nil {
				return err
			}
			entry := &entities.AuditEntry{
				TableName: "transactions",
				RecordID:  id,
				Action:    entities.AuditActionVisibility,
				Field:     strPtr("is_hidden"),
				OldValue:  boolStrPtr(oldByID[id]),
				NewValue:  boolStrPtr(hidden),
				CreatedAt: now,
			}
			if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, entry); err != nil {
				return err
			}
			wrote = true
		}
		if !wrote {
			return nil
		}
		return s.deps.OverlaySvc.RebuildWithTx(ctx, tx)
	})
}

func (s *AnnotationService) BulkAddTags(ctx context.Context, txIDs []int64, tags []string) error {
	return s.bulkTag(ctx, txIDs, tags, true)
}

func (s *AnnotationService) BulkRemoveTags(ctx context.Context, txIDs []int64, tags []string) error {
	return s.bulkTag(ctx, txIDs, tags, false)
}

func (s *AnnotationService) bulkTag(ctx context.Context, txIDs []int64, tags []string, add bool) error {
	if len(txIDs) == 0 {
		return errors.New("no transaction ids provided")
	}
	if len(tags) == 0 {
		return errors.New("no tags provided")
	}
	uniqueIDs := dedupIDs(txIDs)
	uniqueTags := dedupStrings(tags)

	oldTagsByID := make(map[int64][]string, len(uniqueIDs))
	for _, id := range uniqueIDs {
		existing, err := s.deps.TagRepo.ListByTransaction(ctx, id)
		if err != nil {
			return fmt.Errorf("load tags for txn %d: %w", id, err)
		}
		oldTagsByID[id] = existing
	}

	return s.runTx(ctx, func(tx *sql.Tx) error {
		wrote := false
		now := s.deps.Now()
		for _, id := range uniqueIDs {
			old := oldTagsByID[id]
			var next []string
			if add {
				next = append([]string{}, old...)
				for _, t := range uniqueTags {
					if !contains(next, t) {
						next = append(next, t)
					}
				}
			} else {
				next = make([]string, 0, len(old))
				for _, t := range old {
					if !contains(uniqueTags, t) {
						next = append(next, t)
					}
				}
			}
			if stringSlicesEqual(old, next) {
				continue
			}
			if err := s.deps.TagRepo.ClearDBTX(ctx, tx, id); err != nil {
				return fmt.Errorf("clear tags for txn %d: %w", id, err)
			}
			for _, t := range next {
				if err := s.deps.TagRepo.AddDBTX(ctx, tx, id, t); err != nil {
					return fmt.Errorf("add tag %q to txn %d: %w", t, id, err)
				}
			}
			entry := &entities.AuditEntry{
				TableName: "transactions",
				RecordID:  id,
				Action:    entities.AuditActionTag,
				Field:     strPtr("tags"),
				OldValue:  strPtr(joinTags(old)),
				NewValue:  strPtr(joinTags(next)),
				CreatedAt: now,
			}
			if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, entry); err != nil {
				return err
			}
			wrote = true
		}
		if !wrote {
			return nil
		}
		return s.deps.OverlaySvc.RebuildWithTx(ctx, tx)
	})
}

func dedupIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func dedupStrings(ss []string) []string {
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *AnnotationService) Undo(ctx context.Context) error {
	return s.runTx(ctx, func(tx *sql.Tx) error {
		ts, err := s.deps.AuditRepo.FindLatestUndoneTimestampDBTX(ctx, tx)
		if err != nil {
			return fmt.Errorf("find latest undone timestamp: %w", err)
		}
		if ts == "" {
			return errors.New("nothing to undo")
		}

		entries, err := s.deps.AuditRepo.GetByTimestampDBTX(ctx, tx, ts)
		if err != nil {
			return fmt.Errorf("load audit entries: %w", err)
		}
		if len(entries) == 0 {
			return errors.New("nothing to undo")
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM overlay_transactions`); err != nil {
			return fmt.Errorf("clear overlay: %w", err)
		}

		importBatchesToDelete := make(map[int64]bool)

		for _, entry := range entries {
			switch entry.Action {
			case entities.AuditActionImport:
				txn, err := s.deps.TxRepo.GetByIDDBTX(ctx, tx, entry.RecordID)
				if err != nil {
					if errors.Is(err, ports.ErrNotFound) {
						continue
					}
					return fmt.Errorf("get transaction %d: %w", entry.RecordID, err)
				}
				if txn.ImportBatchID != nil {
					importBatchesToDelete[*txn.ImportBatchID] = true
				}
				if err := s.deps.TxRepo.DeleteDBTX(ctx, tx, entry.RecordID); err != nil {
					return fmt.Errorf("delete transaction %d: %w", entry.RecordID, err)
				}

		case entities.AuditActionCategorize:
			var catID *int64
			if entry.OldValue != nil && *entry.OldValue != "" {
				c, err := s.deps.CategoryRepo.GetByNameDBTX(ctx, tx, *entry.OldValue)
				if err != nil {
					return fmt.Errorf("lookup category %q for undo: %w", *entry.OldValue, err)
				}
				catID = &c.ID
			}
			if err := s.deps.TxRepo.SetCategoryDBTX(ctx, tx, entry.RecordID, catID); err != nil {
				return fmt.Errorf("restore category for txn %d: %w", entry.RecordID, err)
			}

			case entities.AuditActionVisibility:
				hidden := false
				if entry.OldValue != nil && *entry.OldValue == "true" {
					hidden = true
				}
				if err := s.deps.TxRepo.SetHiddenDBTX(ctx, tx, entry.RecordID, hidden); err != nil {
					return fmt.Errorf("restore hidden for txn %d: %w", entry.RecordID, err)
				}

			case entities.AuditActionBucket:
				var bucketID *int64
				if entry.OldValue != nil && *entry.OldValue != "" {
					id, err := strconv.ParseInt(*entry.OldValue, 10, 64)
					if err != nil {
						return fmt.Errorf("parse old bucket id %q: %w", *entry.OldValue, err)
					}
					bucketID = &id
				}
				if err := s.deps.TxRepo.SetBucketDBTX(ctx, tx, entry.RecordID, bucketID); err != nil {
					return fmt.Errorf("restore bucket for txn %d: %w", entry.RecordID, err)
				}

			case entities.AuditActionSplit:
				// NewValue is comma-separated child IDs created by the split.
				// Undo deletes them; the parent automatically re-appears in the
				// overlay on rebuild because it no longer has children.
				if entry.NewValue == nil || *entry.NewValue == "" {
					return fmt.Errorf("split audit row missing children list")
				}
				for _, part := range strings.Split(*entry.NewValue, ",") {
					part = strings.TrimSpace(part)
					if part == "" {
						continue
					}
					childID, err := strconv.ParseInt(part, 10, 64)
					if err != nil {
						return fmt.Errorf("parse child id %q: %w", part, err)
					}
					if err := s.deps.TxRepo.DeleteDBTX(ctx, tx, childID); err != nil {
						if errors.Is(err, ports.ErrNotFound) {
							continue
						}
						return fmt.Errorf("delete split child %d: %w", childID, err)
					}
				}

			case entities.AuditActionTag:
				if err := s.deps.TagRepo.ClearDBTX(ctx, tx, entry.RecordID); err != nil {
					return fmt.Errorf("clear tags for txn %d: %w", entry.RecordID, err)
				}
				if entry.OldValue != nil && *entry.OldValue != "" {
					tags := strings.Split(*entry.OldValue, ",")
					for _, tag := range tags {
						tag = strings.TrimSpace(tag)
						if tag != "" {
							if err := s.deps.TagRepo.AddDBTX(ctx, tx, entry.RecordID, tag); err != nil {
								return fmt.Errorf("restore tag %q for txn %d: %w", tag, entry.RecordID, err)
							}
						}
					}
				}

		case entities.AuditActionCategoryRename:
			if entry.OldValue == nil {
				return fmt.Errorf("category_rename audit row missing old name")
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE categories SET name = ? WHERE id = ?`,
				*entry.OldValue, entry.RecordID,
			); err != nil {
				return fmt.Errorf("restore category name %q: %w", *entry.OldValue, err)
			}

		case entities.AuditActionRuleApply:
			// Identical reverse logic to AuditActionCategorize/Bucket:
			// restore the prior value using OldValue / the resolved name.
			// The action may have been triggered by --overwrite, but the
			// stored old/new are still the right things to swap back.
			if entry.Field != nil && *entry.Field == "category" {
				var catID *int64
				if entry.OldValue != nil && *entry.OldValue != "" {
					c, err := s.deps.CategoryRepo.GetByNameDBTX(ctx, tx, *entry.OldValue)
					if err != nil {
						return fmt.Errorf("lookup category %q for undo: %w", *entry.OldValue, err)
					}
					catID = &c.ID
				}
				if err := s.deps.TxRepo.SetCategoryDBTX(ctx, tx, entry.RecordID, catID); err != nil {
					return fmt.Errorf("restore category for txn %d: %w", entry.RecordID, err)
				}
			} else if entry.Field != nil && *entry.Field == "bucket_id" {
				var bucketID *int64
				if entry.OldValue != nil && *entry.OldValue != "" {
					id, err := strconv.ParseInt(*entry.OldValue, 10, 64)
					if err != nil {
						return fmt.Errorf("parse old bucket id %q: %w", *entry.OldValue, err)
					}
					bucketID = &id
				}
				if err := s.deps.TxRepo.SetBucketDBTX(ctx, tx, entry.RecordID, bucketID); err != nil {
					return fmt.Errorf("restore bucket for txn %d: %w", entry.RecordID, err)
				}
			}

			case entities.AuditActionCategoryArchive:
				if _, err := tx.ExecContext(ctx,
					`UPDATE categories SET archived_at = NULL WHERE id = ?`,
					entry.RecordID,
				); err != nil {
					return fmt.Errorf("unarchive category %d: %w", entry.RecordID, err)
				}

			case entities.AuditActionCategoryCreate:
				// No-op: creation is permanent. Reversing a category_create
				// would orphan tx.category_id FKs and the audit log would no
				// longer reproduce the categories table. Archive instead.

			default:
				return fmt.Errorf("cannot undo action %q: unsupported action type", entry.Action)
			}
		}

		for batchID := range importBatchesToDelete {
			if s.deps.BatchRepo != nil {
				if err := s.deps.BatchRepo.DeleteDBTX(ctx, tx, batchID); err != nil {
					return fmt.Errorf("delete empty import batch %d: %w", batchID, err)
				}
			}
		}

		undoEntry := &entities.AuditEntry{
			TableName: "audit_log",
			RecordID:  0,
			Action:    entities.AuditActionUndo,
			OldValue:  &ts,
			CreatedAt: s.deps.Now(),
		}
		if _, err := s.deps.AuditRepo.AppendDBTX(ctx, tx, undoEntry); err != nil {
			return fmt.Errorf("append undo audit entry: %w", err)
		}

		return s.deps.OverlaySvc.RebuildWithTx(ctx, tx)
	})
}

func (s *AnnotationService) resolveBucket(ctx context.Context, name *string, txnCurrency string) (*entities.Bucket, error) {
	if name == nil {
		return nil, nil
	}
	if s.deps.BucketRepo == nil {
		return nil, errors.New("bucket service not configured")
	}
	bucket, err := s.deps.BucketRepo.GetByName(ctx, *name)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, fmt.Errorf("bucket %q not found", *name)
		}
		return nil, fmt.Errorf("lookup bucket: %w", err)
	}
	if bucket.ArchivedAt != nil {
		return nil, fmt.Errorf("bucket %q is archived", *name)
	}
	if bucket.Currency != txnCurrency {
		return nil, fmt.Errorf("bucket %q is %s but transaction is %s", *name, bucket.Currency, txnCurrency)
	}
	return bucket, nil
}

func sameBucketID(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func sameCategoryID(a, b *int64) bool {
	return sameBucketID(a, b)
}

func (s *AnnotationService) resolveCategoryID(ctx context.Context, name string) (*int64, error) {
	if name == "" {
		return nil, nil
	}
	c, err := s.deps.CategoryRepo.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, fmt.Errorf("category %q not found", name)
		}
		return nil, fmt.Errorf("lookup category: %w", err)
	}
	return &c.ID, nil
}

func (s *AnnotationService) categoryNameOrEmpty(ctx context.Context, id *int64) string {
	if id == nil {
		return ""
	}
	c, err := s.deps.CategoryRepo.GetByID(ctx, *id)
	if err != nil {
		return ""
	}
	return c.Name
}

func bucketIDToStringPtr(id *int64) *string {
	if id == nil {
		s := ""
		return &s
	}
	s := strconv.FormatInt(*id, 10)
	return &s
}
