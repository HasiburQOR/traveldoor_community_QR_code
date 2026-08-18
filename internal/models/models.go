// Package models holds the plain data structures shared by the store,
// services and handlers.
package models

import "time"

type User struct {
	ID           int64
	Email        string
	PasswordHash string
	CreatedAt    string
}

type Profile struct {
	ID          int64
	Slug        string
	Name        string
	Subtitle    string
	Description string
	LogoPath    string
	ThemeJSON   string
	Phone       string
	Email       string
	Address     string
	Website     string
	Published   bool
	CreatedAt   string
	UpdatedAt   string
}

type Link struct {
	ID        int64
	ProfileID int64
	Type      string
	Label     string
	URL       string
	Icon      string
	SortOrder int
	Enabled   bool
}

type Session struct {
	ID        string
	UserID    int64
	ExpiresAt time.Time
}

// ProfileStats is the aggregate analytics view for one profile.
type ProfileStats struct {
	Views   int
	Clicks  int
	PerLink map[int64]int
}
