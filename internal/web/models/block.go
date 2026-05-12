package models

import "time"

type EmailBlock struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Category     string    `json:"category"`
	HTML         string    `json:"html"`
	PreviewText  string    `json:"preview_text"`
	BorderRadius int       `json:"border_radius"`
	PaddingV     int       `json:"padding_v"`
	PaddingH     int       `json:"padding_h"`
	Background   string    `json:"background"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type BlockCategory struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	Blocks []EmailBlock `json:"blocks"`
}

type BlockListFilter struct {
	Search   string
	Category string
	Limit    int
	Offset   int
}
