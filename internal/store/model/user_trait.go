package model

import "time"

type UserTrait struct {
	ID              int64
	UserID          int64
	TraitType       string
	NormalizedValue string
	DisplayValue    string
	SourceText      string
	LastConfirmedAt time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
