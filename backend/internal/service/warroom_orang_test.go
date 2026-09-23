package service

import (
	"testing"
	"time"

	"marketingflow/internal/model"
	"marketingflow/internal/repository"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func dbOrangUji(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("buka db: %v", err)
	}
	if err := db.AutoMigrate(&model.WorkItem{}, &model.WorkStep{}, &model.User{}); err != nil {
		t.Fatalf("migrasi: %v", err)
	}
	return db
}

// TestPerformaOrangMembawaBebanDanHasil mengunci keputusan yang membuat panel
// ini adil: satu baris memuat yang MENGGANTUNG sekaligus yang SUDAH
// DITUNTASKAN. Menilai orang dari sisa pekerjaannya saja membuat yang paling
// cepat — dan karena itu kebagian paling banyak — justru terlihat paling buruk.
func TestPerformaOrangMembawaBebanDanHasil(t *testing.T) {
	db := dbOrangUji(t)
	now := time.Now()
	telat := now.Add(-48 * time.Hour)
	segera := now.Add(6 * time.Hour)
	santai := now.Add(10 * 24 * time.Hour)
	baru := now.Add(-3 * time.Hour)
	lama := now.Add(-40 * 24 * time.Hour)

	item := model.WorkItem{Title: "Kampanye Mawar", Alur: model.AlurVideoAd, Stage: model.StageBrief}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("simpan item: %v", err)
	}
	langkah := []model.WorkStep{
		{WorkItemID: item.ID, Code: "B1", Name: "Naskah", Owner: "Copywriter", Status: model.StatusPending, DueDate: &telat},
		{WorkItemID: item.ID, Code: "B2", Name: "Revisi", Owner: "Copywriter", Status: model.StatusInProgress, DueDate: &segera},
		{WorkItemID: item.ID, Code: "B3", Name: "Desain", Owner: "Design Grafis", Status: model.StatusPending, DueDate: &santai},
		// Sudah dituntaskan dalam jendela 30 hari — inilah sisi "hasil".
		{WorkItemID: item.ID, Code: "B4", Name: "Naskah lama", Owner: "Copywriter", Status: model.StatusDone, CompletedAt: &baru},
		// Di luar jendela: tidak boleh ikut dihitung, kalau tidak orang yang
		// sudah dua bulan menganggur tetap terlihat produktif.
		{WorkItemID: item.ID, Code: "B5", Name: "Naskah kuno", Owner: "Copywriter", Status: model.StatusDone, CompletedAt: &lama},
	}
	if err := db.Create(&langkah).Error; err != nil {
		t.Fatalf("simpan langkah: %v", err)
	}
	if err := db.Create(&model.User{
		Name: "Rani", Email: "rani@greenpark.id", PasswordHash: "x",
		Role: model.RoleStaff, Position: "Copywriter",
	}).Error; err != nil {
		t.Fatalf("simpan akun: %v", err)
	}

	orang, err := performaOrang(repository.NewStepRepository(db), repository.NewUserRepository(db))
	if err != nil {
		t.Fatal(err)
	}
	if len(orang) != 2 {
		t.Fatalf("peran = %d, mau 2 (Copywriter & Design Grafis): %+v", len(orang), orang)
	}
	// Yang paling banyak terlambat harus di atas — itu yang ditanyakan di rapat.
	c := orang[0]
	if c.Peran != "Copywriter" {
		t.Fatalf("baris teratas = %q, mau Copywriter", c.Peran)
	}
	if c.Nama != "Rani" {
		t.Fatalf("nama pemegang peran = %q, mau Rani", c.Nama)
	}
	if c.LangkahAktif != 2 || c.Terlambat != 1 || c.SegeraJatuhTempo != 1 {
		t.Fatalf("beban Copywriter = %+v, mau 2 aktif / 1 telat / 1 segera", c)
	}
	if c.Selesai30Hari != 1 {
		t.Fatalf("hasil 30 hari = %d, mau 1 (yang 40 hari lalu tidak dihitung)", c.Selesai30Hari)
	}
	if c.TerakhirSelesai == nil {
		t.Fatal("waktu selesai terakhir hilang")
	}

	d := orang[1]
	if d.Peran != "Design Grafis" || d.Terlambat != 0 || d.LangkahAktif != 1 {
		t.Fatalf("baris kedua = %+v, mau Design Grafis 1 aktif tanpa keterlambatan", d)
	}
	// Belum ada akun dengan jabatan itu: namanya kosong, bukan tertukar dengan
	// nama orang lain.
	if d.Nama != "" {
		t.Fatalf("peran tanpa pemegang seharusnya tanpa nama, dapat %q", d.Nama)
	}
}

// TestPerformaOrangTanpaLangkah: papan kosong menghasilkan daftar kosong, bukan
// nil — di seberang, null terbaca sebagai "gagal membaca".
func TestPerformaOrangTanpaLangkah(t *testing.T) {
	db := dbOrangUji(t)
	orang, err := performaOrang(repository.NewStepRepository(db), repository.NewUserRepository(db))
	if err != nil {
		t.Fatal(err)
	}
	if orang == nil || len(orang) != 0 {
		t.Fatalf("daftar kosong harus [] bukan nil: %+v", orang)
	}
}
