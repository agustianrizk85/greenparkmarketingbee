package model

import "time"

// StepComment adalah satu komentar pada SATU langkah alur konten.
//
// Granularitasnya per LANGKAH, bukan per konten: percakapan di alur ini hampir
// selalu tentang satu langkah tertentu ("revisi desain di A5", "brief kurang
// jelas di A2"). Menaruhnya di konten membuat satu utas panjang yang isinya
// bercampur, dan orang harus membaca ulang untuk tahu bagian mana yang dibahas.
//
// Penulis disimpan sebagai IDENTITAS (email/username SSO) + nama & posisi
// tampilan, BUKAN foreign key ke users: yang ikut berdiskusi bisa direksi atau
// akun lintas divisi yang tidak punya baris user di marketingflow sama sekali.
// Nama & posisi DISALIN saat menulis supaya komentar lama tetap terbaca utuh
// walaupun orangnya kemudian pindah posisi atau akunnya dihapus.
type StepComment struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	StepID uint `gorm:"index;not null" json:"step_id"`

	Author     string `gorm:"size:160;index" json:"author"`
	AuthorName string `gorm:"size:120" json:"author_name"`
	// AuthorRole = posisi di tim (Copywriter, Design Grafis, Video Editor, …).
	// Inilah "tim" dalam percakapan antar tim; kosong bila tidak diketahui.
	AuthorRole string `gorm:"size:64" json:"author_role"`

	Text      string    `gorm:"type:text" json:"text"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}
