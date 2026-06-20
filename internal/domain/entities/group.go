package entities

import "time"

type TransactionGroup struct {
	ID        int64
	Type      string // "transfer" or "reimbursement"
	Name      string
	CreatedAt time.Time
}

type GroupMember struct {
	GroupID       int64
	TransactionID int64
	Role          string
}

const (
	GroupTypeTransfer      = "transfer"
	GroupTypeReimbursement = "reimbursement"
)
