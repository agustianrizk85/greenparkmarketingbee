package model

import "time"

// ProjectLink menautkan NAMA proyek konten ke sebuah proyek Perencanaan.
//
// Ini satu-satunya kunci sambungan lintas divisi modul Marketing: deliverable
// (gambar rumah, render) dibaca dari Perencanaan lewat id ini, dan lahan Permit
// (stok, brosur, PBG) ditemukan lewat kolom perencanaan_project_id yang sama di
// sana. Marketing tidak menyalin data divisi lain - hanya tautannya.
//
// Dicatat per NAMA, bukan per id: proyek konten datang dari Content Plan
// (nama tab spreadsheet) dan tidak punya entitas proyek sendiri di sini.
type ProjectLink struct {
	ProjectName          string    `gorm:"primaryKey;size:200" json:"project_name"`
	PerencanaanProjectID string    `gorm:"size:64;index" json:"perencanaan_project_id"`
	UpdatedBy            uint      `json:"updated_by"`
	UpdatedAt            time.Time `json:"updated_at"`
}
