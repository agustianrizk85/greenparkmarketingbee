package model

import "time"

// Alur is one of the four Marketing workflows from the department flowchart.
//
//	A · Iklan Berbayar – Konten Hardsell / Desain Statis
//	B · Iklan Berbayar – Konten Video
//	C · Konten Organik – Carousel
//	D · Konten Organik – Video / Reels
type Alur string

const (
	AlurHardsell Alur = "A"
	AlurVideoAd  Alur = "B"
	AlurCarousel Alur = "C"
	AlurReels    Alur = "D"
)

// WorkStage tracks how far a work item has progressed through the macro phases
// shared by every alur (brief → produksi → review → approval → distribusi → done).
type WorkStage string

const (
	StageBrief      WorkStage = "brief"
	StageProduksi   WorkStage = "produksi"
	StageReview     WorkStage = "review"
	StageApproval   WorkStage = "approval"
	StageDistribusi WorkStage = "distribusi"
	StageDone       WorkStage = "done"
)

// WorkItem is one run of a Marketing workflow — a single piece of content /
// campaign (e.g. "Iklan Hardsell Cluster Mawar — Juni"). Creating it seeds the
// checklist for the chosen alur only.
type WorkItem struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Title     string    `gorm:"size:180;not null" json:"title"`
	Alur      Alur      `gorm:"size:2;index;not null" json:"alur"`
	Project   string    `gorm:"size:160" json:"project"`  // proyek / cluster perumahan terkait
	Stage     WorkStage `gorm:"size:24;not null;default:brief" json:"stage"`
	CreatedBy uint      `json:"created_by"`
	Creator   *User     `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Sync provenance — set when the item is ingested from the Content Plan
	// spreadsheet. Source is empty for items created manually in the app.
	Source      string     `gorm:"size:32;index" json:"source,omitempty"`            // e.g. "content-plan"
	SourceKey   string     `gorm:"size:48;uniqueIndex" json:"source_key,omitempty"`  // idempotency key (project|date|title)
	SourceTab   string     `gorm:"size:64" json:"source_tab,omitempty"`              // exact sheet tab, e.g. "Copywrite LHL"
	ContentType string     `gorm:"size:48" json:"content_type,omitempty"`            // raw calendar label (Softsell Instagram, …)
	PlannedDate *time.Time `json:"planned_date,omitempty"`                           // scheduled upload date from the plan
	Brief       string     `gorm:"type:text" json:"brief,omitempty"`
	Caption     string     `gorm:"type:text" json:"caption,omitempty"`

	Steps []WorkStep `gorm:"foreignKey:WorkItemID" json:"steps,omitempty"`
}
