package entities

import "time"

type ImportBatch struct {
	ID            int64
	SourceFile    string
	SourceProfile string
	RowCount      int
	InsertedCount int
	SkippedCount  int
	CreatedAt     time.Time
}
