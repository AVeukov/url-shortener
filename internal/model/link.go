package model

import "time"

type Link struct {
	Code        string     `json:"code"`
	OriginalURL string     `json:"url"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	ClickCount  int64      `json:"click_count"`
}

func (l Link) Expired(now time.Time) bool {
	return l.ExpiresAt != nil && !l.ExpiresAt.After(now)
}
