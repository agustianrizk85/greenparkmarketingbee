package repository

import (
	"errors"
	"time"

	"marketingflow/internal/model"

	"gorm.io/gorm"
)

type StepRepository struct {
	db *gorm.DB
}

func NewStepRepository(db *gorm.DB) *StepRepository {
	return &StepRepository{db: db}
}

// LangkahBerjalan memulangkan langkah pertama yang BELUM selesai untuk setiap
// konten — dipakai papan untuk menaruh kartu di kolom langkahnya.
//
// Satu query untuk semua konten, bukan satu per kartu: papan menampilkan
// puluhan kartu sekaligus, dan menanyakannya satu per satu membuat pembukaan
// papan mengirim puluhan permintaan.
func (r *StepRepository) LangkahBerjalan() (map[uint][2]string, error) {
	var rows []struct {
		WorkItemID uint
		Code       string
		Name       string
	}
	if err := r.db.Table("work_steps").
		Select("work_item_id, code, name").
		Where("status <> ?", "done").
		Order("work_item_id asc, sequence asc").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[uint][2]string, len(rows))
	for _, x := range rows {
		// Baris pertama per konten = langkah berjalan (sudah terurut sequence).
		if _, ada := out[x.WorkItemID]; !ada {
			out[x.WorkItemID] = [2]string{x.Code, x.Name}
		}
	}
	return out, nil
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
	if err := r.isiMeta([]*model.WorkStep{&s}); err != nil {
		return nil, err
	}
	return &s, nil
}

// Save menyimpan langkah BESERTA isian metadatanya.
//
// Keduanya di dalam satu transaksi: kalau isian tersimpan sementara langkahnya
// gagal (atau sebaliknya), yang terlihat di layar adalah data yang saling
// bertentangan — dan tidak ada galat yang memberi tahu.
func (r *StepRepository) Save(s *model.WorkStep) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(s).Error; err != nil {
			return err
		}
		return gantiMeta(tx, s.ID, s.Metadata)
	})
}

// ByWorkItem returns every step of a work item ordered by sequence.
func (r *StepRepository) ByWorkItem(workItemID uint) ([]model.WorkStep, error) {
	var steps []model.WorkStep
	if err := r.db.Where("work_item_id = ?", workItemID).Order("sequence asc").Find(&steps).Error; err != nil {
		return nil, err
	}
	ptr := make([]*model.WorkStep, len(steps))
	for i := range steps {
		ptr[i] = &steps[i]
	}
	return steps, r.isiMeta(ptr)
}

/* ---- isian metadata (tabel work_step_meta) ---- */

// isiMeta mengisi field Metadata tiap langkah dari tabel work_step_meta.
//
// SATU kueri untuk semua langkah, bukan satu per langkah: papan menampilkan
// puluhan langkah sekaligus, dan menanyakannya satu per satu mengubah satu
// pembukaan layar jadi puluhan perjalanan ke database — alasan yang sama persis
// dengan LangkahBerjalan di atas.
func (r *StepRepository) isiMeta(steps []*model.WorkStep) error {
	if len(steps) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(steps))
	for _, s := range steps {
		ids = append(ids, s.ID)
	}
	var rows []model.WorkStepMeta
	if err := r.db.Where("work_step_id IN ?", ids).Find(&rows).Error; err != nil {
		return err
	}
	per := make(map[uint]map[string]string, len(steps))
	for _, m := range rows {
		if per[m.WorkStepID] == nil {
			per[m.WorkStepID] = map[string]string{}
		}
		per[m.WorkStepID][m.Kunci] = m.Nilai
	}
	for _, s := range steps {
		// Langkah tanpa isian tetap diberi peta KOSONG, bukan nil: layar
		// membacanya sebagai objek, dan `null` di tempat objek memaksa tiap
		// pemanggil memeriksanya lebih dulu.
		if m := per[s.ID]; m != nil {
			s.Metadata = m
		} else {
			s.Metadata = map[string]string{}
		}
	}
	return nil
}

// gantiMeta menulis ulang seluruh isian satu langkah: yang lama dihapus, yang
// baru dimasukkan. Bentuk "ganti semua" dipilih karena itulah yang dikirim
// layar — ia mengirim peta utuh, bukan perubahan per kunci — jadi menyimpannya
// sebagian justru meninggalkan kunci yang sudah dihapus orang.
func gantiMeta(tx *gorm.DB, stepID uint, meta map[string]string) error {
	if err := tx.Where("work_step_id = ?", stepID).Delete(&model.WorkStepMeta{}).Error; err != nil {
		return err
	}
	if len(meta) == 0 {
		return nil
	}
	rows := make([]model.WorkStepMeta, 0, len(meta))
	for k, v := range meta {
		rows = append(rows, model.WorkStepMeta{WorkStepID: stepID, Kunci: k, Nilai: v})
	}
	return tx.Create(&rows).Error
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
	if err != nil {
		return nil, err
	}
	// Scan TIDAK menjalankan hook GORM, jadi isian metadata harus diambil
	// sendiri di sini — lihat catatan di model.WorkStep.Metadata.
	ptr := make([]*model.WorkStep, len(rows))
	for i := range rows {
		ptr[i] = &rows[i].WorkStep
	}
	return rows, r.isiMeta(ptr)
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
	if err != nil {
		return nil, err
	}
	// Sama seperti OpenSteps: Scan melewati hook, dan LAYAR INILAH yang
	// menyunting isian metadata. Kalau lupa, isiannya tampak kosong padahal
	// tersimpan — lalu tertimpa kosong begitu orangnya menekan simpan.
	ptr := make([]*model.WorkStep, len(rows))
	for i := range rows {
		ptr[i] = &rows[i].WorkStep
	}
	return rows, r.isiMeta(ptr)
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

// LangkahSelesaiSejak mengembalikan langkah yang DISELESAIKAN sejak waktu
// tertentu — bahan "berapa yang benar-benar dituntaskan orang ini".
//
// Dipisah dari OpenSteps karena keduanya menjawab pertanyaan yang berbeda:
// OpenSteps adalah beban yang masih menggantung, ini adalah hasil kerja. Panel
// performa butuh keduanya; menilai orang hanya dari sisa pekerjaannya membuat
// yang paling produktif justru terlihat paling buruk.
func (r *StepRepository) LangkahSelesaiSejak(sejak time.Time) ([]OpenStep, error) {
	var rows []OpenStep
	err := r.db.
		Table("work_steps").
		Select("work_steps.*, work_items.title as work_item_title").
		Joins("JOIN work_items ON work_items.id = work_steps.work_item_id").
		Where("work_steps.status = ? AND work_steps.completed_at >= ?", model.StatusDone, sejak).
		Order("work_steps.completed_at desc").
		Scan(&rows).Error
	return rows, err
}
