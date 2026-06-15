package model

import (
	"time"

	"gorm.io/datatypes"
)

// StepStatus is the checklist state of a single workflow step.
type StepStatus string

const (
	StatusPending    StepStatus = "pending"
	StatusInProgress StepStatus = "in_progress"
	StatusDone       StepStatus = "done"
)

// WorkStep is one item in a Marketing alur checklist (e.g. A2 "Copywriter buat
// brief"). The same generic shape is reused for every alur (A..D): the PIC owner
// is captured per step, paid-ads budget steps require a budget amount, approval
// steps are gated to the Kepala Departemen, and structured links (Drive, iCloud,
// Meta Ads, jadwal posting) live in the flexible Metadata field so new alur can
// be added without schema changes.
type WorkStep struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	WorkItemID uint   `gorm:"index;not null" json:"work_item_id"`
	Code       string `gorm:"size:8;index;not null" json:"code"`  // e.g. "A1"
	Alur       string `gorm:"size:2;index;not null" json:"alur"`  // e.g. "A"
	Name       string `gorm:"size:240;not null" json:"name"`
	Sequence   int    `gorm:"not null" json:"sequence"`
	Phase      string `gorm:"size:24;index;not null" json:"phase"` // brief/produksi/review/approval/distribusi

	// Owner is the PIC role responsible for this step (Copywriter, Design Grafis,
	// Videografer, Video Editor, Talent, Social Media Specialist, Digital
	// Marketing, Kepala Departemen Marketing).
	Owner string `gorm:"size:48" json:"owner"`
	// CollabDept names a supporting department for cross-team steps
	// (Perencanaan / Keuangan / Sales).
	CollabDept string `gorm:"size:48" json:"collab_dept"`

	Status StepStatus `gorm:"size:16;not null;default:pending" json:"status"`

	// Business rules captured from the flowchart.
	IsApproval        bool   `json:"is_approval"`         // "kirim final ke Kepala Departemen untuk approval"
	RequiresBudget    bool   `json:"requires_budget"`     // pengajuan biaya iklan / top up saldo
	BudgetLabel       string `gorm:"size:48" json:"budget_label"`
	NotifyDepartments bool   `json:"notify_departments"`  // lintas departemen (Perencanaan / Keuangan / Sales)

	// SLA / deadline tracking.
	SLADays int        `json:"sla_days"`
	DueDate *time.Time `json:"due_date"`

	// Captured values.
	BudgetAmount int64  `json:"budget_amount"` // rupiah, integer to avoid float rounding
	Notes        string `gorm:"type:text" json:"notes"`

	// Flexible structured data per step (links: brief, footage iCloud, hasil
	// desain/video, Meta Ads, caption, jadwal posting).
	Metadata datatypes.JSON `gorm:"type:jsonb" json:"metadata"`

	CompletedBy *uint      `json:"completed_by"`
	CompletedAt *time.Time `json:"completed_at"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Documents []Document `gorm:"foreignKey:WorkStepID" json:"documents,omitempty"`
}
