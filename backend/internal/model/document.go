package model

import "time"

// Document is an uploaded file attached to a workflow step (hasil desain, draft
// video, footage, screenshot campaign, dll.) — and to its parent work item.
type Document struct {
	ID         uint  `gorm:"primaryKey" json:"id"`
	WorkItemID uint  `gorm:"index;not null" json:"work_item_id"`
	WorkStepID *uint `gorm:"index" json:"work_step_id"`

	DocType      string `gorm:"size:64;not null" json:"doc_type"` // label: "Hasil Desain", "Draft Video", ...
	OriginalName string `gorm:"size:255;not null" json:"original_name"`
	StoredName   string `gorm:"size:255;not null" json:"stored_name"`
	Path         string `gorm:"size:512;not null" json:"-"`
	MimeType     string `gorm:"size:128" json:"mime_type"`
	SizeBytes    int64  `json:"size_bytes"`

	UploadedBy uint      `json:"uploaded_by"`
	CreatedAt  time.Time `json:"created_at"`
}
