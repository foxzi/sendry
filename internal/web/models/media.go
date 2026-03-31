package models

import "time"

type MediaFile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	OrigName  string    `json:"orig_name"`
	MimeType  string    `json:"mime_type"`
	Size      int64     `json:"size"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

type MediaListFilter struct {
	Search string
	Limit  int
	Offset int
}
