package model

import "time"

type UserProfile struct {
	UserID                     int64
	NameValue                  string
	NameConfirmedAt            time.Time
	GenderValue                string
	GenderConfirmedAt          time.Time
	AgeValue                   string
	AgeConfirmedAt             time.Time
	JobValue                   string
	JobConfirmedAt             time.Time
	CurrentFocusValue          string
	CurrentFocusConfirmedAt    time.Time
	HobbyValue                 string
	HobbyConfirmedAt           time.Time
	LocationValue              string
	LocationConfirmedAt        time.Time
	AffiliationValue           string
	AffiliationConfirmedAt     time.Time
	LastRequestedSlot          string
	LastRequestedUserTurnCount int
	CollectionPausedUntilTurn  int
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}
