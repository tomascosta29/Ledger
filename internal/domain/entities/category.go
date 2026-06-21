package entities

import "time"

type Category struct {
	ID          int64
	Name        string
	Description string
	ArchivedAt  *time.Time
	CreatedAt   time.Time
}
