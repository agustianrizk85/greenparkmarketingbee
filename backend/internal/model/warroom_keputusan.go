package model

import "time"

// KEPUTUSAN WAR ROOM — usulan AI yang DITERIMA CEO, disimpan jadi perintah.
//
// KENAPA DISIMPAN. Tanpa catatan, war room berhenti di "PUTUSKAN": usulan
// dibacakan, disetujui di ruangan, lalu hilang. Yang membuat layar ini ada
// gunanya justru dua langkah sesudahnya — PANTAU dan VERIFIKASI — dan keduanya
// menuntut perintahnya masih ada minggu depan beserta cara mengukurnya.
//
// KENAPA MENYIMPAN TARGET DAN BUKTI APA ADANYA. TargetID adalah id kampanye,
// kreatif, proyek, atau percakapan di sumber aslinya; Bukti adalah angka yang
// dipakai saat keputusan diambil. Keduanya disimpan agar verifikasi bisa
// membandingkan keadaan hari ini dengan keadaan saat perintah dikeluarkan —
// bukan dengan ingatan orang.
type WarroomKeputusan struct {
	ID uint `gorm:"primaryKey" json:"id"`

	Sev      string `gorm:"size:16" json:"sev"` // merah | kuning | hijau
	Judul    string `gorm:"size:200" json:"judul"`
	Tindakan string `gorm:"size:40" json:"tindakan"` // naikkan budget | hentikan | …

	TargetTipe string `gorm:"size:20" json:"targetTipe"` // kampanye | kreatif | proyek | kanal | lead
	TargetID   string `gorm:"size:120" json:"targetId"`
	TargetNama string `gorm:"size:200" json:"targetNama"`

	Alasan            string `gorm:"type:text" json:"alasan"`
	Bukti             string `gorm:"type:text" json:"bukti"` // JSON array angka saat keputusan diambil
	UangDipertaruhkan string `gorm:"size:80" json:"uangDipertaruhkan"`
	DampakDiharapkan  string `gorm:"type:text" json:"dampakDiharapkan"`

	PIC              string `gorm:"size:80" json:"pic"`
	Tenggat          string `gorm:"size:40" json:"tenggat"`
	MetrikVerifikasi string `gorm:"type:text" json:"metrikVerifikasi"`

	// CekPada = tanggal perintah ini seharusnya dinilai hasilnya. Tanggal, bukan
	// durasi: "cek 3 hari lagi" berubah artinya setiap kali dibaca.
	CekPada time.Time `json:"cekPada"`

	// Status "terbuka" berarti belum dinilai. Selain itu: berhasil | belum jelas
	// | gagal — persis tiga kemungkinan yang boleh dilaporkan AI, supaya
	// laporannya tidak pernah menemukan status yang tak dikenal.
	Status     string `gorm:"size:20;index" json:"status"`
	HasilBukti string `gorm:"type:text" json:"hasilBukti"`

	DibuatOleh       string     `gorm:"size:120" json:"dibuatOleh"`
	DibuatPada       time.Time  `json:"dibuatPada"`
	DiverifikasiOleh string     `gorm:"size:120" json:"diverifikasiOleh"`
	DiverifikasiPada *time.Time `json:"diverifikasiPada"`
}

// TableName menjaga nama tabel tetap eksplisit.
func (WarroomKeputusan) TableName() string { return "warroom_keputusan" }

// Status keputusan.
const (
	KeputusanTerbuka    = "terbuka"
	KeputusanBerhasil   = "berhasil"
	KeputusanBelumJelas = "belum jelas"
	KeputusanGagal      = "gagal"
)

// StatusKeputusanSah melaporkan apakah s salah satu status yang dikenal.
func StatusKeputusanSah(s string) bool {
	switch s {
	case KeputusanTerbuka, KeputusanBerhasil, KeputusanBelumJelas, KeputusanGagal:
		return true
	}
	return false
}

// TindakanKeputusan adalah satu-satunya tindakan yang boleh dicatat — daftar
// yang sama dengan yang boleh diusulkan AI. Dibatasi supaya perintah bisa
// dihitung per jenis, dan supaya tidak lahir tindakan baru yang tak seorang pun
// tahu cara memverifikasinya.
var TindakanKeputusan = []string{
	"naikkan budget",
	"turunkan budget",
	"hentikan",
	"ganti kreatif",
	"perbaiki penargetan",
	"balas lead",
	"alihkan budget",
	"kumpulkan data",
}

// TindakanSah melaporkan apakah t ada di TindakanKeputusan.
func TindakanSah(t string) bool {
	for _, x := range TindakanKeputusan {
		if x == t {
			return true
		}
	}
	return false
}
