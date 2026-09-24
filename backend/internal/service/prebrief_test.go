package service

import (
	"testing"

	"marketingflow/internal/dto"
	"marketingflow/internal/model"
	"marketingflow/internal/repository"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func svcUji(t *testing.T) (*WorkItemService, *StepService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("buka db: %v", err)
	}
	if err := db.AutoMigrate(&model.WorkItem{}, &model.WorkStep{}, &model.User{}, &model.Document{}, &model.WorkStepMeta{}); err != nil {
		t.Fatalf("migrasi: %v", err)
	}
	items := repository.NewWorkItemRepository(db)
	steps := repository.NewStepRepository(db)
	itemSvc := NewWorkItemService(items, steps)
	return itemSvc, NewStepService(steps, itemSvc), db
}

// Satu pre-brief untuk tiga orang harus menghasilkan TIGA konten terpisah —
// masing-masing dengan penerima, brief, dan ceklis alurnya sendiri.
func TestPreBriefMembuatSatuKontenPerPenerima(t *testing.T) {
	itemSvc, _, db := svcUji(t)
	penerima := []model.User{
		{Name: "Hanif", Email: "hanif@greenpark.id", Position: "Design Grafis", Role: model.RoleStaff},
		{Name: "Rina", Email: "rina@greenpark.id", Position: "Video Editor", Role: model.RoleStaff},
		{Name: "Dodi", Email: "dodi@greenpark.id", Position: "Copywriter", Role: model.RoleStaff},
	}
	for i := range penerima {
		if err := db.Create(&penerima[i]).Error; err != nil {
			t.Fatalf("seed akun: %v", err)
		}
	}

	// Brief BERBEDA per orang; yang terakhir dikosongkan supaya jatuh ke brief umum.
	briefPer := []string{"Buat 3 varian visual.", "Durasi 15 detik, subtitle wajib.", ""}
	req := dto.PreBriefRequest{
		Title: "Iklan Hardsell Cluster Mawar", Alur: model.AlurHardsell,
		Project: "Lahan Baru", Brief: "Fokus ke harga promo bulan ini.",
		Assignees: []dto.PreBriefTujuan{
			{UserID: penerima[0].ID, Brief: briefPer[0]},
			{UserID: penerima[1].ID, Brief: briefPer[1]},
			{UserID: penerima[2].ID, Brief: briefPer[2]},
		},
	}
	pasangan := []PenerimaBrief{
		{User: penerima[0], Brief: briefPer[0]},
		{User: penerima[1], Brief: briefPer[1]},
		{User: penerima[2], Brief: briefPer[2]},
	}
	hasil, err := itemSvc.CreatePreBrief(req, 1, pasangan)
	if err != nil {
		t.Fatalf("pre-brief: %v", err)
	}
	if len(hasil) != 3 {
		t.Fatalf("konten dibuat = %d, mau 3", len(hasil))
	}

	var jumlah int64
	db.Model(&model.WorkItem{}).Count(&jumlah)
	if jumlah != 3 {
		t.Fatalf("konten tersimpan = %d, mau 3", jumlah)
	}
	for i, it := range hasil {
		if it.AssignedTo == nil || *it.AssignedTo != penerima[i].ID {
			t.Fatalf("konten %d tidak menunjuk penerimanya: %+v", i, it.AssignedTo)
		}
		// Nama & posisi DISALIN, bukan dirujuk — itu inti penugasannya.
		if it.AssigneeName != penerima[i].Name || it.AssigneePosition != penerima[i].Position {
			t.Fatalf("identitas penerima tidak disalin: %q / %q", it.AssigneeName, it.AssigneePosition)
		}
		// Brief per orang: yang diisi memakai miliknya sendiri, yang dikosongkan
		// jatuh ke brief umum.
		mau := briefPer[i]
		if mau == "" {
			mau = req.Brief
		}
		if it.Brief != mau {
			t.Fatalf("brief konten %d = %q, mau %q", i, it.Brief, mau)
		}
		var langkah int64
		db.Model(&model.WorkStep{}).Where("work_item_id = ?", it.ID).Count(&langkah)
		if langkah == 0 {
			t.Fatalf("konten %d tidak punya ceklis", i)
		}
	}
}

// Tahap Review & Revisi hanya boleh disentuh Copywriter dan Kepala Departemen.
func TestLangkahReviewHanyaCopywriterDanKadep(t *testing.T) {
	itemSvc, stepSvc, db := svcUji(t)
	item := &model.WorkItem{Title: "Tes", Alur: model.AlurHardsell, Stage: model.StageBrief}
	if _, err := itemSvc.Create(dto.CreateWorkItemRequest{Title: item.Title, Alur: item.Alur}, 1); err != nil {
		t.Fatalf("buat konten: %v", err)
	}
	var langkahReview model.WorkStep
	if err := db.Where("phase = ?", FaseReview).First(&langkahReview).Error; err != nil {
		t.Fatalf("langkah review tidak ada di katalog: %v", err)
	}

	jadi := model.StatusDone
	req := dto.UpdateStepRequest{Status: &jadi}

	if _, err := stepSvc.Update(langkahReview.ID, req, 9, model.RoleStaff, "Design Grafis"); err == nil {
		t.Fatal("Design Grafis boleh mengubah langkah review, seharusnya ditolak")
	}
	if _, err := stepSvc.Update(langkahReview.ID, req, 9, model.RoleStaff, OwnerCopywriter); err != nil {
		t.Fatalf("Copywriter ditolak di langkah review: %v", err)
	}
	if _, err := stepSvc.Update(langkahReview.ID, req, 9, model.RoleKadep, ""); err != nil {
		t.Fatalf("Kepala Departemen ditolak di langkah review: %v", err)
	}
}

// Tahap lain tidak ikut terkunci — penjagaan ini khusus Review & Revisi.
func TestLangkahSelainReviewTetapBebas(t *testing.T) {
	itemSvc, stepSvc, db := svcUji(t)
	if _, err := itemSvc.Create(dto.CreateWorkItemRequest{Title: "Tes", Alur: model.AlurHardsell}, 1); err != nil {
		t.Fatalf("buat konten: %v", err)
	}
	var langkah model.WorkStep
	if err := db.Where("phase <> ? AND is_approval = ?", FaseReview, false).First(&langkah).Error; err != nil {
		t.Fatalf("langkah non-review tidak ada: %v", err)
	}
	jalan := model.StatusInProgress
	if _, err := stepSvc.Update(langkah.ID, dto.UpdateStepRequest{Status: &jalan}, 9, model.RoleStaff, "Design Grafis"); err != nil {
		t.Fatalf("langkah biasa ikut terkunci: %v", err)
	}
}
