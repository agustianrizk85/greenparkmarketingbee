package repository

// ISIAN METADATA LANGKAH — dulu satu kolom jsonb, sekarang tabel work_step_meta.
//
// Yang dikunci di sini bukan "datanya tersimpan" (itu bagian yang mudah),
// melainkan tiga kegagalan yang semuanya DIAM:
//
//  1. jalur baca yang memakai Scan melewati hook GORM, jadi isiannya tampak
//     kosong padahal ada — lalu tertimpa kosong begitu orangnya menekan simpan;
//  2. menyimpan peta baru tanpa menghapus yang lama meninggalkan kunci yang
//     sudah dibuang orang;
//  3. menghapus konten tanpa menghapus isian langkahnya meninggalkan baris
//     yatim yang dipungut kembali oleh id langkah yang dipakai ulang.

import (
	"testing"

	"marketingflow/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func dbMeta(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:stepmeta?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("buka db: %v", err)
	}
	if err := db.AutoMigrate(&model.WorkItem{}, &model.WorkStep{},
		&model.WorkStepMeta{}, &model.Document{}, &model.StepComment{}); err != nil {
		t.Fatalf("migrasi: %v", err)
	}
	for _, m := range []any{&model.WorkStepMeta{}, &model.WorkStep{}, &model.WorkItem{}} {
		if err := db.Where("1 = 1").Delete(m).Error; err != nil {
			t.Fatalf("kosongkan: %v", err)
		}
	}
	return db
}

func langkahUji(t *testing.T, db *gorm.DB, owner string) *model.WorkStep {
	t.Helper()
	item := model.WorkItem{Title: "Konten Uji", Alur: model.AlurHardsell}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("buat konten: %v", err)
	}
	s := model.WorkStep{WorkItemID: item.ID, Code: "A1", Name: "Brief", Owner: owner,
		Sequence: 1, Status: model.StatusPending}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("buat langkah: %v", err)
	}
	return &s
}

func TestIsianTersimpanDanTerbacaKembali(t *testing.T) {
	db := dbMeta(t)
	r := NewStepRepository(db)
	s := langkahUji(t, db, "Desainer")

	s.Metadata = map[string]string{"link_brief": "https://x/brief", "tanggal_shooting": "2026-10-01"}
	if err := r.Save(s); err != nil {
		t.Fatalf("simpan: %v", err)
	}

	lagi, err := r.FindByID(s.ID)
	if err != nil {
		t.Fatalf("baca: %v", err)
	}
	if lagi.Metadata["link_brief"] != "https://x/brief" ||
		lagi.Metadata["tanggal_shooting"] != "2026-10-01" {
		t.Fatalf("isian tidak kembali utuh: %#v", lagi.Metadata)
	}
}

// Kunci yang DIBUANG orang harus benar-benar hilang. Layar mengirim peta utuh,
// bukan perubahan per kunci — jadi kunci yang tidak ikut dikirim artinya dihapus.
func TestKunciYangDibuangTidakKembali(t *testing.T) {
	db := dbMeta(t)
	r := NewStepRepository(db)
	s := langkahUji(t, db, "Desainer")

	s.Metadata = map[string]string{"link_brief": "a", "link_desain": "b"}
	if err := r.Save(s); err != nil {
		t.Fatalf("simpan awal: %v", err)
	}
	s.Metadata = map[string]string{"link_brief": "a"} // link_desain dibuang
	if err := r.Save(s); err != nil {
		t.Fatalf("simpan ulang: %v", err)
	}

	lagi, err := r.FindByID(s.ID)
	if err != nil {
		t.Fatalf("baca: %v", err)
	}
	if _, masih := lagi.Metadata["link_desain"]; masih {
		t.Error("kunci yang dibuang masih tersimpan")
	}
	if lagi.Metadata["link_brief"] != "a" {
		t.Error("kunci yang dipertahankan ikut hilang")
	}
}

// JALUR SCAN — inilah jebakannya.
//
// ByOwner dan OpenSteps memakai Scan, yang TIDAK menjalankan hook GORM. Kalau
// isian tidak diambil sendiri di sana, layar "Tugas Saya" dan tampilan lapangan
// menampilkan isian kosong padahal datanya ada — lalu menimpanya dengan kosong
// begitu orangnya menyimpan.
func TestJalurScanIkutMembawaIsian(t *testing.T) {
	db := dbMeta(t)
	r := NewStepRepository(db)
	s := langkahUji(t, db, "Talent & Videografer")

	s.Metadata = map[string]string{"link_footage_icloud": "https://icloud/x"}
	if err := r.Save(s); err != nil {
		t.Fatalf("simpan: %v", err)
	}

	milikSaya, err := r.ByOwner("Talent")
	if err != nil {
		t.Fatalf("ByOwner: %v", err)
	}
	if len(milikSaya) != 1 {
		t.Fatalf("mau 1 langkah, dapat %d", len(milikSaya))
	}
	if milikSaya[0].Metadata["link_footage_icloud"] != "https://icloud/x" {
		t.Errorf("ByOwner tidak membawa isian: %#v", milikSaya[0].Metadata)
	}

	terbuka, err := r.OpenSteps()
	if err != nil {
		t.Fatalf("OpenSteps: %v", err)
	}
	if len(terbuka) != 1 || terbuka[0].Metadata["link_footage_icloud"] != "https://icloud/x" {
		t.Errorf("OpenSteps tidak membawa isian: %#v", terbuka)
	}
}

// Langkah tanpa isian harus memulangkan peta KOSONG, bukan nil: layar
// membacanya sebagai objek, dan `null` di tempat objek memaksa tiap pemanggil
// memeriksanya lebih dulu.
func TestLangkahTanpaIsianBukanNil(t *testing.T) {
	db := dbMeta(t)
	r := NewStepRepository(db)
	s := langkahUji(t, db, "Desainer")

	lagi, err := r.FindByID(s.ID)
	if err != nil {
		t.Fatalf("baca: %v", err)
	}
	if lagi.Metadata == nil {
		t.Error("Metadata nil — seharusnya peta kosong")
	}
}

// Menghapus konten harus membawa serta isian langkahnya. Baris yatim akan
// dipungut kembali oleh id langkah yang dipakai ulang, dan isian milik konten
// yang sudah dihapus muncul di konten yang baru.
func TestHapusKontenIkutMembuangIsian(t *testing.T) {
	db := dbMeta(t)
	r := NewStepRepository(db)
	wi := NewWorkItemRepository(db)
	s := langkahUji(t, db, "Desainer")

	s.Metadata = map[string]string{"link_brief": "a"}
	if err := r.Save(s); err != nil {
		t.Fatalf("simpan: %v", err)
	}
	if _, _, err := wi.DeleteItem(s.WorkItemID); err != nil {
		t.Fatalf("hapus konten: %v", err)
	}

	var sisa int64
	if err := db.Model(&model.WorkStepMeta{}).Where("work_step_id = ?", s.ID).Count(&sisa).Error; err != nil {
		t.Fatalf("hitung: %v", err)
	}
	if sisa != 0 {
		t.Errorf("%d isian tertinggal yatim setelah kontennya dihapus", sisa)
	}
}
