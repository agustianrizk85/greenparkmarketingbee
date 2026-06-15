package model

import "time"

// Role enumerates the system roles for the Marketing department workflow.
//   - kadep  : Kepala Departemen Marketing (approver of final content & budget).
//   - staff  : tim internal (Copywriter, Design Grafis, Videografer, Video Editor,
//              Talent, Social Media Specialist, Digital Marketing) — eksekutor langkah.
//   - viewer : read-only.
type Role string

const (
	RoleKadep  Role = "kadep"
	RoleStaff  Role = "staff"
	RoleViewer Role = "viewer"
)

// User is an application account. Role is the capability tier (approve/edit/read)
// while Position is the PIC label from the flowchart (Copywriter, Design Grafis,
// …) — it matches WorkStep.Owner so "tugas saya" can be derived per account.
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"size:120;not null" json:"name"`
	Email        string    `gorm:"size:160;uniqueIndex;not null" json:"email"`
	PasswordHash string    `gorm:"size:255;not null" json:"-"`
	Role         Role      `gorm:"size:32;not null" json:"role"`
	Position     string    `gorm:"size:64" json:"position"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
