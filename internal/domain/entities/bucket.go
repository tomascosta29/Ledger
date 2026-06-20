package entities

import "time"

type Bucket struct {
	ID                    int64
	Name                  string
	Currency              string
	MonthlyAllocationMinor int64
	ArchivedAt            *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}
