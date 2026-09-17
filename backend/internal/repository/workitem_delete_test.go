package repository

import (
	"testing"

	"marketingflow/internal/model"
)

// Menghapus satu konten harus ikut membawa langkah, komentar langkah, dan baris
// dokumennya — dan TIDAK menyentuh konten lain. Komentar menempel pada LANGKAH,
// bukan pada kontennya, jadi itu bagian yang paling mudah tertinggal.
func TestHapusSatuKontenMembawaAnaknya(t *testing.T) {
	db := dbUji(t)
	if err := db.AutoMigrate(&model.Document{}, &model.StepComment{}); err != nil {
		t.Fatalf("migrasi tambahan: %v", err)
	}
	r := NewWorkItemRepository(db)

	buat := func(judul string) *model.WorkItem {
		item := &model.WorkItem{Title: judul, Alur: model.AlurVideoAd, Stage: model.StageBrief}
		langkah := []model.WorkStep{{Code: "B1", Alur: "B", Name: "Brief", Phase: "brief"}}
		if err := r.CreateWithSteps(item, langkah); err != nil {
			t.Fatalf("simpan %q: %v", judul, err)
		}
		return item
	}
	dibuang := buat("Tes")
	disimpan := buat("Video 3")

	var langkahDibuang model.WorkStep
	if err := db.Where("work_item_id = ?", dibuang.ID).First(&langkahDibuang).Error; err != nil {
		t.Fatalf("langkah tidak terbentuk: %v", err)
	}
	if err := db.Create(&model.StepComment{StepID: langkahDibuang.ID, Author: "a", Text: "cek"}).Error; err != nil {
		t.Fatalf("komentar: %v", err)
	}
	if err := db.Create(&model.Document{
		WorkItemID: dibuang.ID, DocType: "Hasil Desain", OriginalName: "a.png",
		StoredName: "a.png", Path: "uploads/a.png",
	}).Error; err != nil {
		t.Fatalf("dokumen: %v", err)
	}

	counts, berkas, err := r.DeleteItem(dibuang.ID)
	if err != nil {
		t.Fatalf("hapus: %v", err)
	}
	if counts.WorkItems != 1 || counts.WorkSteps != 1 || counts.Documents != 1 {
		t.Fatalf("hitungan = %+v, mau 1/1/1", counts)
	}
	if len(berkas) != 1 || berkas[0] != "uploads/a.png" {
		t.Fatalf("path berkas = %v, mau [uploads/a.png]", berkas)
	}

	var sisaKomentar, sisaLangkah, sisaItem int64
	db.Model(&model.StepComment{}).Count(&sisaKomentar)
	db.Model(&model.WorkStep{}).Count(&sisaLangkah)
	db.Model(&model.WorkItem{}).Count(&sisaItem)
	if sisaKomentar != 0 {
		t.Fatalf("komentar yatim tersisa: %d", sisaKomentar)
	}
	// Konten satunya harus utuh: satu item + satu langkahnya.
	if sisaItem != 1 || sisaLangkah != 1 {
		t.Fatalf("konten lain ikut terhapus: item=%d langkah=%d", sisaItem, sisaLangkah)
	}
	if _, err := r.FindByID(disimpan.ID); err != nil {
		t.Fatalf("konten yang tidak dihapus hilang: %v", err)
	}
}
