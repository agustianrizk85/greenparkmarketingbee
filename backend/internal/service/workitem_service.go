package service

import (
	"errors"
	"strings"
	"time"

	"marketingflow/internal/dto"
	"marketingflow/internal/model"
	"marketingflow/internal/repository"
)

type WorkItemService struct {
	items *repository.WorkItemRepository
	steps *repository.StepRepository
}

func NewWorkItemService(items *repository.WorkItemRepository, steps *repository.StepRepository) *WorkItemService {
	return &WorkItemService{items: items, steps: steps}
}

// Create persists a new work item and seeds its checklist from the alur catalog.
func (s *WorkItemService) Create(req dto.CreateWorkItemRequest, createdBy uint) (*model.WorkItem, error) {
	item := &model.WorkItem{
		Title:     req.Title,
		Alur:      req.Alur,
		Project:   req.Project,
		Stage:     model.StageBrief,
		CreatedBy: createdBy,
	}
	steps := BuildSteps(req.Alur, time.Now().UTC())
	if err := s.items.CreateWithSteps(item, steps); err != nil {
		return nil, err
	}
	return item, nil
}

// UpdateBrief menyimpan catatan kartu. Yang boleh: penerima briefnya, yang
// membuatnya, dan kepala departemen. Penerima ikut karena catatan itu miliknya
// — ia yang menambahkan hasil rapat, tautan, dan koreksi saat mengerjakan.
func (s *WorkItemService) UpdateBrief(id uint, brief string, oleh uint, peran model.Role) error {
	item, err := s.items.FindByID(id)
	if err != nil {
		return err
	}
	pemilik := item.CreatedBy == oleh || (item.AssignedTo != nil && *item.AssignedTo == oleh)
	if peran != model.RoleKadep && !pemilik {
		return ErrTidakBerhak
	}
	return s.items.UpdateBrief(id, brief)
}

// ErrStageTidakDikenal menolak kolom di luar lima yang ada.
var ErrStageTidakDikenal = errors.New("kolom tidak dikenal")

// kolomSah — lima kolom papan konten. Sengaja daftar tertutup: kolom karangan
// membuat kartunya lenyap dari papan tanpa jejak.
var kolomSah = map[model.WorkStage]bool{
	model.StageBrief: true, model.StageProduksi: true, model.StageReview: true,
	model.StageApproval: true, model.StageDistribusi: true,
}

// Pindahkan memindahkan kartu ke kolom lain.
//
// Kolom "Review & Revisi" dijaga sama seperti langkahnya: hanya Copywriter dan
// Kepala Departemen yang boleh memasukkan kartu ke sana ATAU mengeluarkannya —
// kalau hanya salah satunya dijaga, siapa pun bisa menghindari penilaian dengan
// menggeser kartunya keluar sendiri.
func (s *WorkItemService) Pindahkan(id uint, stage model.WorkStage, oleh uint, peran model.Role, posisi string) error {
	if !kolomSah[stage] {
		return ErrStageTidakDikenal
	}
	item, err := s.items.FindByID(id)
	if err != nil {
		return err
	}
	pemilik := item.CreatedBy == oleh || (item.AssignedTo != nil && *item.AssignedTo == oleh)
	if peran != model.RoleKadep && !pemilik {
		return ErrTidakBerhak
	}
	menyentuhReview := stage == model.StageReview || item.Stage == model.StageReview
	if menyentuhReview && peran != model.RoleKadep && posisi != OwnerCopywriter {
		return ErrReviewRole
	}
	return s.items.PindahkanKartu(id, stage)
}

// ErrPenerimaTidakAda dipulangkan bila id penerima tidak cocok dengan akun mana
// pun — lebih baik gagal daripada membuat konten tanpa pemilik.
var ErrPenerimaTidakAda = errors.New("akun tujuan tidak ditemukan")

// PenerimaBrief memasangkan satu akun dengan brief yang ditujukan kepadanya.
type PenerimaBrief struct {
	User  model.User
	Brief string
}

// CreatePreBrief membuat SATU konten untuk SETIAP penerima, masing-masing
// membawa briefnya sendiri. Penerima yang briefnya dikosongkan memakai brief
// umum — supaya arahan yang memang sama tidak perlu diketik berulang.
func (s *WorkItemService) CreatePreBrief(req dto.PreBriefRequest, createdBy uint, penerima []PenerimaBrief) ([]model.WorkItem, error) {
	if len(penerima) == 0 {
		return nil, ErrPenerimaTidakAda
	}
	out := make([]model.WorkItem, 0, len(penerima))
	for _, p := range penerima {
		u := p.User
		id := u.ID
		brief := strings.TrimSpace(p.Brief)
		if brief == "" {
			brief = req.Brief
		}
		item := &model.WorkItem{
			Title:            req.Title,
			Alur:             req.Alur,
			Project:          req.Project,
			Brief:            brief,
			Stage:            model.StageBrief,
			CreatedBy:        createdBy,
			AssignedTo:       &id,
			AssigneeName:     u.Name,
			AssigneePosition: u.Position,
		}
		if err := s.items.CreateWithSteps(item, BuildSteps(req.Alur, time.Now().UTC())); err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, nil
}

// BuildSteps instantiates the checklist for an alur, anchoring each step's due
// date to `anchor` + its SLA. The Content Plan sync anchors to the planned upload
// date so warnings reflect the schedule; manual creation anchors to now.
func BuildSteps(alur model.Alur, anchor time.Time) []model.WorkStep {
	templates := CatalogFor(alur)
	steps := make([]model.WorkStep, 0, len(templates))
	for i, t := range templates {
		step := t.toModel(0, i+1)
		if step.SLADays > 0 && !anchor.IsZero() {
			due := anchor.AddDate(0, 0, step.SLADays)
			step.DueDate = &due
		}
		steps = append(steps, step)
	}
	return steps
}

func (s *WorkItemService) List() ([]model.WorkItem, error) {
	items, err := s.items.List()
	if err != nil {
		return nil, err
	}
	// Langkah berjalan ikut dikirim supaya papan bisa menaruh kartu di kolom
	// langkahnya tanpa menarik seluruh ceklis tiap konten.
	berjalan, err := s.steps.LangkahBerjalan()
	if err != nil {
		return nil, err
	}
	for i := range items {
		if v, ada := berjalan[items[i].ID]; ada {
			items[i].CurrentStepCode, items[i].CurrentStepName = v[0], v[1]
		}
	}
	return items, nil
}

// ErrTidakBerhak dipulangkan saat yang menghapus bukan pemilik kontennya dan
// bukan kepala departemen.
var ErrTidakBerhak = errors.New("hanya pembuat konten atau kepala departemen yang boleh menghapus")

// Delete menghapus SATU konten. Aturan izinnya sengaja lebih longgar daripada
// "Hapus Semua Data" (khusus kadep): membersihkan konten percobaan milik
// sendiri adalah pekerjaan sehari-hari, dan memaksanya lewat kadep hanya
// membuat papan penuh sampah yang tidak berani dihapus siapa pun.
func (s *WorkItemService) Delete(id, olehID uint, peran model.Role) (repository.ResetCounts, []string, error) {
	item, err := s.items.FindByID(id)
	if err != nil {
		return repository.ResetCounts{}, nil, err
	}
	if peran != model.RoleKadep && item.CreatedBy != olehID {
		return repository.ResetCounts{}, nil, ErrTidakBerhak
	}
	return s.items.DeleteItem(id)
}

// DeleteAll wipes every work item, step and document. Accounts are preserved.
func (s *WorkItemService) DeleteAll() (repository.ResetCounts, error) {
	return s.items.DeleteAllWorkData()
}

func (s *WorkItemService) Get(id uint) (*model.WorkItem, error) {
	return s.items.FindByID(id)
}

// Progress computes the checklist completion summary for the dashboard.
func (s *WorkItemService) Progress(id uint) (*dto.WorkItemProgress, error) {
	counts, err := s.steps.CountByStatus(id)
	if err != nil {
		return nil, err
	}
	var total, done int64
	for status, n := range counts {
		total += n
		if status == model.StatusDone {
			done += n
		}
	}
	pct := 0
	if total > 0 {
		pct = int(done * 100 / total)
	}
	return &dto.WorkItemProgress{
		WorkItemID: id,
		Total:      total,
		Done:       done,
		Percentage: pct,
		ByStatus:   counts,
	}, nil
}

// RecomputeStage advances the work item stage to the phase of the earliest
// not-done step (or "done" when every step is complete). Called after a step
// status change so the dashboard stepper reflects real progress.
func (s *WorkItemService) RecomputeStage(workItemID uint) error {
	steps, err := s.steps.ByWorkItem(workItemID)
	if err != nil {
		return err
	}
	stage := model.StageDone
	for _, st := range steps {
		if st.Status != model.StatusDone {
			stage = model.WorkStage(st.Phase)
			break
		}
	}
	return s.items.UpdateStage(workItemID, stage)
}
