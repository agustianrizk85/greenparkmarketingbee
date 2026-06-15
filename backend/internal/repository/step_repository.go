package repository

import (
	"errors"

	"marketingflow/internal/model"

	"gorm.io/gorm"
)

type StepRepository struct {
	db *gorm.DB
}

func NewStepRepository(db *gorm.DB) *StepRepository {
	return &StepRepository{db: db}
}

func (r *StepRepository) FindByID(id uint) (*model.WorkStep, error) {
	var s model.WorkStep
	err := r.db.Preload("Documents").First(&s, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *StepRepository) Save(s *model.WorkStep) error {
	return r.db.Save(s).Error
}

// ByWorkItem returns every step of a work item ordered by sequence.
func (r *StepRepository) ByWorkItem(workItemID uint) ([]model.WorkStep, error) {
	var steps []model.WorkStep
	err := r.db.Where("work_item_id = ?", workItemID).Order("sequence asc").Find(&steps).Error
	return steps, err
}

// OpenStep is a not-yet-done step joined with its work item, for early warnings.
type OpenStep struct {
	model.WorkStep
	WorkItemTitle string `json:"work_item_title"`
}

// OpenSteps returns every step that is not "done", with the work item title, so
// the early-warning engine can evaluate SLA breaches and missing inputs.
func (r *StepRepository) OpenSteps() ([]OpenStep, error) {
	var rows []OpenStep
	err := r.db.
		Table("work_steps").
		Select("work_steps.*, work_items.title as work_item_title").
		Joins("JOIN work_items ON work_items.id = work_steps.work_item_id").
		Where("work_steps.status <> ?", model.StatusDone).
		Order("work_steps.due_date asc").
		Scan(&rows).Error
	return rows, err
}

// MineStep is a step joined with its work item context, for the per-PIC board.
type MineStep struct {
	model.WorkStep
	WorkItemTitle string `json:"work_item_title"`
	WorkItemAlur  string `json:"work_item_alur"`
}

// ByOwner returns every step whose Owner contains the given position label
// (e.g. "Talent" matches owner "Talent & Videografer"), across all work items —
// powering the "Tugas Saya" kanban and the field-team mobile view.
func (r *StepRepository) ByOwner(position string) ([]MineStep, error) {
	var rows []MineStep
	err := r.db.
		Table("work_steps").
		Select("work_steps.*, work_items.title as work_item_title, work_items.alur as work_item_alur").
		Joins("JOIN work_items ON work_items.id = work_steps.work_item_id").
		Where("work_steps.owner LIKE ?", "%"+position+"%").
		Order("work_steps.due_date asc").
		Scan(&rows).Error
	return rows, err
}

// CountByStatus returns how many steps a work item has in each status, used by
// the dashboard progress summary.
func (r *StepRepository) CountByStatus(workItemID uint) (map[model.StepStatus]int64, error) {
	type row struct {
		Status model.StepStatus
		Count  int64
	}
	var rows []row
	err := r.db.Model(&model.WorkStep{}).
		Select("status, count(*) as count").
		Where("work_item_id = ?", workItemID).
		Group("status").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := map[model.StepStatus]int64{}
	for _, r := range rows {
		out[r.Status] = r.Count
	}
	return out, nil
}
