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

// PindahkanKartu menyimpan kolom kartu. Dipisah dari UpdateStage yang dipakai
// RecomputeStage supaya niatnya terbaca: yang ini perpindahan oleh ORANG.
func (r *WorkItemRepository) PindahkanKartu(id uint, stage model.WorkStage) error {
	return r.db.Model(&model.WorkItem{}).Where("id = ?", id).Update("stage", stage).Error
}

// UpdateBrief menyimpan CATATAN kartu (isi brief). Hanya kolom itu yang
// disentuh: judul, alur, dan tahapnya punya jalurnya sendiri, dan menyimpan
// seluruh baris di sini berarti menimpa perubahan orang lain yang kebetulan
// menggeser kartunya pada saat yang sama.
func (r *WorkItemRepository) UpdateBrief(id uint, brief string) error {
	return r.db.Model(&model.WorkItem{}).Where("id = ?", id).Update("brief", brief).Error
}

// DeleteItem menghapus SATU konten beserta seluruh anaknya dalam satu
// transaksi, dan mengembalikan path berkas lampirannya supaya pemanggil bisa
// membersihkan disk setelah barisnya benar-benar hilang.
//
// Urutannya anak dulu: komentar langkah -> dokumen -> langkah -> kontennya.
// Komentar menempel pada LANGKAH (bukan konten), jadi ia harus dihapus lewat
// daftar id langkah — tanpa itu ia jadi yatim dan ikut terbaca di utas langkah
// milik konten lain yang kebetulan memakai ulang id yang sama.
func (r *WorkItemRepository) DeleteItem(id uint) (ResetCounts, []string, error) {
	var counts ResetCounts
	var berkas []string
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var stepIDs []uint
		if err := tx.Model(&model.WorkStep{}).Where("work_item_id = ?", id).Pluck("id", &stepIDs).Error; err != nil {
			return err
		}
		if len(stepIDs) > 0 {
			if err := tx.Where("step_id IN ?", stepIDs).Delete(&model.StepComment{}).Error; err != nil {
				return err
			}
			// Isian metadata juga menempel pada LANGKAH, dengan alasan yang sama
			// persis seperti komentar: tanpa dihapus di sini ia jadi yatim, dan
			// id langkah yang dipakai ulang akan memungutnya kembali — isian
			// milik konten yang sudah dihapus muncul di konten yang baru.
			if err := tx.Where("work_step_id IN ?", stepIDs).Delete(&model.WorkStepMeta{}).Error; err != nil {
				return err
			}
		}

		if err := tx.Model(&model.Document{}).Where("work_item_id = ?", id).Pluck("path", &berkas).Error; err != nil {
			return err
		}
		res := tx.Where("work_item_id = ?", id).Delete(&model.Document{})
		if res.Error != nil {
			return res.Error
		}
		counts.Documents = res.RowsAffected

		res = tx.Where("work_item_id = ?", id).Delete(&model.WorkStep{})
		if res.Error != nil {
			return res.Error
		}
		counts.WorkSteps = res.RowsAffected

		res = tx.Where("id = ?", id).Delete(&model.WorkItem{})
		if res.Error != nil {
			return res.Error
		}
		counts.WorkItems = res.RowsAffected
		return nil
	})
	return counts, berkas, err
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
