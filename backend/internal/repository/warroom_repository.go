package repository

import (
	"context"
	"time"

	"marketingflow/internal/model"

	"gorm.io/gorm"
)

// WarroomRepository menyimpan perintah yang lahir dari war room.
type WarroomRepository struct {
	db *gorm.DB
}

func NewWarroomRepository(db *gorm.DB) *WarroomRepository {
	return &WarroomRepository{db: db}
}

// Catat menyimpan satu perintah baru.
func (r *WarroomRepository) Catat(k *model.WarroomKeputusan) error {
	return r.db.Create(k).Error
}

// Daftar mengembalikan perintah terbaru lebih dulu. status kosong = semua.
func (r *WarroomRepository) Daftar(status string, batas int) ([]model.WarroomKeputusan, error) {
	q := r.db.Order("dibuat_pada desc")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if batas > 0 {
		q = q.Limit(batas)
	}
	var out []model.WarroomKeputusan
	err := q.Find(&out).Error
	return out, err
}

// Ambil membaca satu perintah.
func (r *WarroomRepository) Ambil(id uint) (model.WarroomKeputusan, error) {
	var k model.WarroomKeputusan
	err := r.db.First(&k, id).Error
	return k, err
}

// Verifikasi menutup satu perintah dengan hasilnya.
func (r *WarroomRepository) Verifikasi(id uint, status, bukti, oleh string, pada time.Time) error {
	return r.db.Model(&model.WarroomKeputusan{}).Where("id = ?", id).Updates(map[string]any{
		"status":            status,
		"hasil_bukti":       bukti,
		"diverifikasi_oleh": oleh,
		"diverifikasi_pada": pada,
	}).Error
}

// Hapus membuang satu perintah (salah catat).
func (r *WarroomRepository) Hapus(id uint) error {
	return r.db.Delete(&model.WarroomKeputusan{}, id).Error
}

// Ringkas menghitung perintah terbuka, yang sudah lewat tanggal ceknya, dan
// yang selesai sepekan terakhir.
//
// "Telat" dihitung SISTEM, bukan ditaksir AI: angka inilah yang memicu briefing
// baru, jadi ia harus sama persis bagi layar dan bagi AI.
func (r *WarroomRepository) Ringkas(ctx context.Context, now time.Time) (terbuka, telat, selesai7 int64, err error) {
	db := r.db.WithContext(ctx).Model(&model.WarroomKeputusan{})
	if err = db.Where("status = ?", model.KeputusanTerbuka).Count(&terbuka).Error; err != nil {
		return
	}
	if err = r.db.WithContext(ctx).Model(&model.WarroomKeputusan{}).
		Where("status = ? AND cek_pada < ?", model.KeputusanTerbuka, now).Count(&telat).Error; err != nil {
		return
	}
	err = r.db.WithContext(ctx).Model(&model.WarroomKeputusan{}).
		Where("status <> ? AND diverifikasi_pada >= ?", model.KeputusanTerbuka, now.AddDate(0, 0, -7)).
		Count(&selesai7).Error
	return
}

// JatuhTempo mengembalikan perintah terbuka yang sudah waktunya dinilai, plus
// yang baru selesai — persis bahan KEPUTUSAN_TERAKHIR yang dikirim ke AI.
func (r *WarroomRepository) JatuhTempo(now time.Time, batas int) ([]model.WarroomKeputusan, error) {
	var out []model.WarroomKeputusan
	err := r.db.Where("status = ? OR (status <> ? AND diverifikasi_pada >= ?)",
		model.KeputusanTerbuka, model.KeputusanTerbuka, now.AddDate(0, 0, -7)).
		Order("cek_pada asc").Limit(batas).Find(&out).Error
	return out, err
}
