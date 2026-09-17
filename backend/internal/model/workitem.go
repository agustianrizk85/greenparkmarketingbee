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
	// Penerima brief. Diisi lewat Pre-Brief: satu konten milik SATU orang, dan
	// nama serta posisinya DISALIN saat penugasan — sama seperti komentar —
	// supaya kartunya tetap terbaca utuh kalau orangnya pindah posisi atau
	// akunnya kemudian dihapus.
	AssignedTo       *uint  `gorm:"index" json:"assigned_to,omitempty"`
	AssigneeName     string `gorm:"size:120" json:"assignee_name,omitempty"`
	AssigneePosition string `gorm:"size:64" json:"assignee_position,omitempty"`

	Source      string     `gorm:"size:32;index" json:"source,omitempty"`            // e.g. "content-plan"
	// SourceKey WAJIB nullable, bukan string kosong: indeks uniknya menganggap
	// "" sebagai nilai biasa, jadi item KEDUA yang dibuat manual di aplikasi
	// (yang kuncinya sama-sama kosong) akan ditolak "UNIQUE constraint failed".
	// NULL boleh berulang di SQLite maupun Postgres, dan itulah arti yang benar:
	// item manual memang tidak punya kunci idempotensi.
	SourceKey   *string    `gorm:"size:48;uniqueIndex" json:"source_key,omitempty"`  // idempotency key (project|date|title)
	SourceTab   string     `gorm:"size:64" json:"source_tab,omitempty"`              // exact sheet tab, e.g. "Copywrite LHL"
	ContentType string     `gorm:"size:48" json:"content_type,omitempty"`            // raw calendar label (Softsell Instagram, …)
	PlannedDate *time.Time `json:"planned_date,omitempty"`                           // scheduled upload date from the plan
	Brief       string     `gorm:"type:text" json:"brief,omitempty"`
	Caption     string     `gorm:"type:text" json:"caption,omitempty"`

	Steps []WorkStep `gorm:"foreignKey:WorkItemID" json:"steps,omitempty"`

	// LANGKAH BERJALAN — langkah pertama yang belum selesai. Tidak disimpan
	// (gorm:"-"), dihitung saat daftar dibaca: ia turunan dari status langkah,
	// dan menyimpannya berarti ada dua kebenaran yang bisa berselisih.
	// Kosong = seluruh langkahnya sudah selesai.
	CurrentStepCode string `gorm:"-" json:"current_step_code,omitempty"`
	CurrentStepName string `gorm:"-" json:"current_step_name,omitempty"`
}
