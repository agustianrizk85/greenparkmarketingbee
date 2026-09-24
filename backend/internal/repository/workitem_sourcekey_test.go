package repository

import (
	"testing"

	"marketingflow/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// dbUji membuka SQLite in-memory yang sudah dimigrasi — satu basis data per tes,
// supaya baris tes lain tidak ikut terhitung.
func dbUji(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("buka db: %v", err)
	}
	if err := db.AutoMigrate(&model.WorkItem{}, &model.WorkStep{}, &model.WorkStepMeta{}); err != nil {
		t.Fatalf("migrasi: %v", err)
	}
	return db
}

// Item yang dibuat MANUAL tidak punya kunci idempotensi, dan harus boleh lebih
// dari satu. Dulu kuncinya string kosong sehingga indeks uniknya menolak item
// manual KEDUA dengan "UNIQUE constraint failed: work_items.source_key".
func TestDuaItemManualBolehTanpaKunciSumber(t *testing.T) {
	r := NewWorkItemRepository(dbUji(t))

	for _, judul := range []string{"Tes", "Tes lagi"} {
		item := &model.WorkItem{Title: judul, Alur: model.AlurVideoAd, Stage: model.StageBrief}
		if err := r.CreateWithSteps(item, nil); err != nil {
			t.Fatalf("simpan %q: %v", judul, err)
		}
	}

	var n int64
	if err := r.db.Model(&model.WorkItem{}).Count(&n).Error; err != nil {
		t.Fatalf("hitung: %v", err)
	}
	if n != 2 {
		t.Fatalf("item tersimpan = %d, mau 2", n)
	}
}

// Kunci yang BENAR-BENAR ada tetap harus unik: itu gunanya indeks tadi, supaya
// sinkron Content Plan yang diulang tidak menggandakan barisnya.
func TestKunciSumberYangSamaDitolak(t *testing.T) {
	r := NewWorkItemRepository(dbUji(t))
	kunci := "vertihome|2026-09-17|iklan"

	pertama := &model.WorkItem{Title: "Dari sheet", Alur: model.AlurVideoAd, Stage: model.StageBrief, SourceKey: &kunci}
	if err := r.CreateWithSteps(pertama, nil); err != nil {
		t.Fatalf("item pertama gagal: %v", err)
	}
	kedua := &model.WorkItem{Title: "Dari sheet (ulang)", Alur: model.AlurVideoAd, Stage: model.StageBrief, SourceKey: &kunci}
	if err := r.CreateWithSteps(kedua, nil); err == nil {
		t.Fatal("kunci sumber kembar diterima, seharusnya ditolak")
	}

	// Kunci NULL tidak boleh ikut terbaca sebagai "sudah pernah disinkron".
	manual := &model.WorkItem{Title: "Manual", Alur: model.AlurVideoAd, Stage: model.StageBrief}
	if err := r.CreateWithSteps(manual, nil); err != nil {
		t.Fatalf("item manual gagal: %v", err)
	}
	ada, err := r.ExistingSourceKeys()
	if err != nil {
		t.Fatalf("baca kunci: %v", err)
	}
	if len(ada) != 1 || !ada[kunci] {
		t.Fatalf("kunci tersimpan = %+v, mau hanya %q", ada, kunci)
	}
}
