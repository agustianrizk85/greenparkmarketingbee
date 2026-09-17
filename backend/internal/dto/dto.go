package dto

import (
	"time"

	"marketingflow/internal/model"
)

// --- Auth ---

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token     string      `json:"token"`
	ExpiresAt string      `json:"expires_at"`
	User      *model.User `json:"user"`
}

// --- WorkItem ---

type CreateWorkItemRequest struct {
	Title   string     `json:"title" binding:"required"`
	Alur    model.Alur `json:"alur" binding:"required,oneof=A B C D"`
	Project string     `json:"project"`
}

// PreBriefRequest membuka pekerjaan baru untuk BEBERAPA orang sekaligus: satu
// konten dibuat per penerima, masing-masing dengan ceklis alurnya sendiri.
// Dipisah dari CreateWorkItemRequest karena artinya memang berbeda — yang ini
// menugaskan, yang itu hanya mencatat.
type PreBriefRequest struct {
	Title   string     `json:"title" binding:"required"`
	Alur    model.Alur `json:"alur" binding:"required,oneof=A B C D"`
	Project string     `json:"project"`
	// Brief umum: dipakai untuk penerima yang tidak diberi brief sendiri.
	// Boleh kosong bila semuanya diisi satu per satu.
	Brief     string           `json:"brief"`
	Assignees []PreBriefTujuan `json:"assignees" binding:"required,min=1"`
}

// UpdateStageRequest — memindahkan kartu ke kolom lain.
type UpdateStageRequest struct {
	Stage model.WorkStage `json:"stage" binding:"required"`
}

// UpdateBriefRequest — menyunting CATATAN kartu konten.
type UpdateBriefRequest struct {
	Brief string `json:"brief"`
}

// PreBriefTujuan — satu penerima beserta briefnya sendiri. Brief menempel pada
// PENERIMA, bukan pada kirimannya: Copywriter dan Design Grafis mengerjakan hal
// yang berbeda dari campaign yang sama, jadi arahannya memang berbeda.
type PreBriefTujuan struct {
	UserID uint   `json:"user_id" binding:"required"`
	Brief  string `json:"brief"`
}

// --- Step ---

// UpdateStepRequest carries partial updates. Pointer fields allow distinguishing
// "not provided" from a zero value (important for budget = 0 / clearing notes).
type UpdateStepRequest struct {
	Status       *model.StepStatus `json:"status"`
	BudgetAmount *int64            `json:"budget_amount"`
	Notes        *string           `json:"notes"`
	Metadata     map[string]any    `json:"metadata"`
	SLADays      *int              `json:"sla_days"`
	DueDate      *time.Time        `json:"due_date"`
}

// --- Dashboard ---

type WorkItemProgress struct {
	WorkItemID uint                       `json:"work_item_id"`
	Total      int64                      `json:"total"`
	Done       int64                      `json:"done"`
	Percentage int                        `json:"percentage"`
	ByStatus   map[model.StepStatus]int64 `json:"by_status"`
}
