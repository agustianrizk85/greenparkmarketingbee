package repository

import (
	"errors"

	"marketingflow/internal/model"

	"gorm.io/gorm"
)

type WorkItemRepository struct {
	db *gorm.DB
}

func NewWorkItemRepository(db *gorm.DB) *WorkItemRepository {
	return &WorkItemRepository{db: db}
}

// CreateWithSteps persists the work item and its seeded steps in one transaction.
func (r *WorkItemRepository) CreateWithSteps(w *model.WorkItem, steps []model.WorkStep) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(w).Error; err != nil {
			return err
		}
		for i := range steps {
			steps[i].WorkItemID = w.ID
		}
		if len(steps) > 0 {
			if err := tx.Create(&steps).Error; err != nil {
				return err
			}
		}
		w.Steps = steps
		return nil
	})
}

func (r *WorkItemRepository) List() ([]model.WorkItem, error) {
	var items []model.WorkItem
	err := r.db.Order("created_at desc").Find(&items).Error
	return items, err
}

func (r *WorkItemRepository) FindByID(id uint) (*model.WorkItem, error) {
	var w model.WorkItem
	err := r.db.
		Preload("Steps", func(db *gorm.DB) *gorm.DB { return db.Order("work_steps.sequence asc") }).
		Preload("Steps.Documents").
		First(&w, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *WorkItemRepository) UpdateStage(id uint, stage model.WorkStage) error {
	return r.db.Model(&model.WorkItem{}).Where("id = ?", id).Update("stage", stage).Error
}
