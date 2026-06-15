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

	Steps []WorkStep `gorm:"foreignKey:WorkItemID" json:"steps,omitempty"`
}
