package model

import "time"

type SpecialDay struct {
	ID                int64
	Day               time.Time
	Name              string
	KindCode          string
	KindLabel         string
	IsHoliday         bool
	Seq               int
	IsMajorHoliday    bool
	MajorHolidayGroup string
	SourcePayload     []byte
	FetchedAt         time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
