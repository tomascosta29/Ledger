package entities

import "time"

type Rule struct {
	ID               int64
	Name             string
	Priority         int
	MatchPartner     *string
	MatchDescription *string
	MatchAmountMin   *int64
	MatchAmountMax   *int64
	SetCategory      *string
	SetBucketID      *int64
	AddTags          []string
	Enabled          bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (r Rule) MatchesAmount(amountMinor int64) bool {
	if r.MatchAmountMin != nil && amountMinor < *r.MatchAmountMin {
		return false
	}
	if r.MatchAmountMax != nil && amountMinor > *r.MatchAmountMax {
		return false
	}
	return true
}
