package entities

import "time"

type TransactionGroup struct {
	ID        int64
	Name      string
	CreatedAt time.Time
}

type GroupMember struct {
	GroupID       int64
	TransactionID int64
	Role          string
}
