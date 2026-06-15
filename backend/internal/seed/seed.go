package seed

import (
	"log"

	"marketingflow/internal/config"
	"marketingflow/internal/model"
	"marketingflow/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

// Accounts seeds the default accounts (KADEP, Staff, Viewer) if the user table
// is empty. Passwords come from the environment so they can be rotated.
func Accounts(users *repository.UserRepository, cfg *config.Config) error {
	count, err := users.Count()
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	// One account per role in the flowchart: Kepala Departemen (approver) + each
	// member of "Tim Internal Marketing" + the read-only viewer. Position mirrors
	// WorkStep.Owner so a person's steps can be filtered later.
	defaults := []struct {
		name     string
		email    string
		role     model.Role
		position string
		password string
	}{
		{"Kepala Departemen Marketing", "kadep@greenpark.id", model.RoleKadep, "Kepala Departemen Marketing", cfg.SeedKadepPassword},
		{"Copywriter", "copywriter@greenpark.id", model.RoleStaff, "Copywriter", cfg.SeedStaffPassword},
		{"Talent", "talent@greenpark.id", model.RoleStaff, "Talent", cfg.SeedStaffPassword},
		{"Videografer", "videografer@greenpark.id", model.RoleStaff, "Videografer", cfg.SeedStaffPassword},
		{"Video Editor", "editor@greenpark.id", model.RoleStaff, "Video Editor", cfg.SeedStaffPassword},
		{"Design Grafis", "design@greenpark.id", model.RoleStaff, "Design Grafis", cfg.SeedStaffPassword},
		{"Social Media Specialist", "sosmed@greenpark.id", model.RoleStaff, "Social Media Specialist", cfg.SeedStaffPassword},
		{"Digital Marketing", "digital@greenpark.id", model.RoleStaff, "Digital Marketing", cfg.SeedStaffPassword},
		{"Viewer", "viewer@greenpark.id", model.RoleViewer, "Viewer", cfg.SeedViewerPassword},
	}

	for _, d := range defaults {
		hash, err := bcrypt.GenerateFromPassword([]byte(d.password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		u := &model.User{Name: d.name, Email: d.email, Role: d.role, Position: d.position, PasswordHash: string(hash)}
		if err := users.Create(u); err != nil {
			return err
		}
		log.Printf("seeded account: %s (%s · %s)", d.email, d.role, d.position)
	}
	return nil
}
