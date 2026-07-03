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

// ResetCounts reports how many rows each delete touched.
type ResetCounts struct {
	WorkItems int64 `json:"work_items"`
	WorkSteps int64 `json:"work_steps"`
	Documents int64 `json:"documents"`
}

// DeleteAllWorkData wipes every work item, step and document in one transaction.
// Accounts (users) and Meta connections/config are left untouched.
func (r *WorkItemRepository) DeleteAllWorkData() (ResetCounts, error) {
	var counts ResetCounts
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// Children first to respect FK ordering on stricter drivers.
		res := tx.Exec("DELETE FROM documents")
		if res.Error != nil {
			return res.Error
		}
		counts.Documents = res.RowsAffected

		res = tx.Exec("DELETE FROM work_steps")
		if res.Error != nil {
			return res.Error
		}
		counts.WorkSteps = res.RowsAffected

		res = tx.Exec("DELETE FROM work_items")
		if res.Error != nil {
			return res.Error
		}
		counts.WorkItems = res.RowsAffected
		return nil
	})
	return counts, err
}

// UpdateSyncedMeta refreshes the descriptive fields of an already-imported item
// (keyed by source key) so a re-sync reflects edits made in the sheet — without
// touching its checklist steps, stage or progress. Alur is intentionally NOT
// updated (changing it would require re-seeding steps).
func (r *WorkItemRepository) UpdateSyncedMeta(sourceKey string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.Model(&model.WorkItem{}).Where("source_key = ?", sourceKey).Updates(fields).Error
}

// ExistingSourceKeys returns the set of source keys already stored, so a sync can
// skip items it has imported before (idempotency).
func (r *WorkItemRepository) ExistingSourceKeys() (map[string]bool, error) {
	var keys []string
	if err := r.db.Model(&model.WorkItem{}).
		Where("source_key <> ''").
		Pluck("source_key", &keys).Error; err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}
	return set, nil
}
