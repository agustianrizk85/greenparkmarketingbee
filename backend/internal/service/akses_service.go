package service

import (
	"strings"

	"marketingflow/internal/model"
	"marketingflow/internal/repository"
)

// AKSES — menjawab "siapa orang ini" untuk keputusan hak.
//
// Peran (kadep/staff/viewer) ikut di token, tapi POSISI (Copywriter, Design
// Grafis, Video Editor, …) tidak. Padahal dua aturan alur kerja ini justru
// digantung pada posisi: yang boleh menulis Pre-Brief, dan yang boleh menilai di
// tahap Review & Revisi. Jadi posisinya dibaca dari basis data di sini — satu
// tempat, supaya tidak ada dua jawaban berbeda untuk orang yang sama.

type AksesService struct{ users *repository.UserRepository }

func NewAksesService(users *repository.UserRepository) *AksesService {
	return &AksesService{users: users}
}

// Posisi mencari posisi pemakai. Akun asli Marketing dikenali lewat id; akun SSO
// lintas divisi masuk dengan id 0, jadi ia dicari lewat surelnya. Tidak ketemu =
// "" (bukan galat): pemakai tanpa posisi hanya berarti tidak punya hak khusus,
// bukan permintaan yang rusak.
func (s *AksesService) Posisi(id uint, email string) string {
	if s == nil || s.users == nil {
		return ""
	}
	if id != 0 {
		if u, err := s.users.FindByID(id); err == nil && u != nil {
			return strings.TrimSpace(u.Position)
		}
	}
	if e := strings.TrimSpace(email); e != "" {
		if u, err := s.users.FindByEmail(e); err == nil && u != nil {
			return strings.TrimSpace(u.Position)
		}
	}
	return ""
}

// Akun mengambil akun-akun tujuan Pre-Brief menurut id. Id yang tidak ditemukan
// TIDAK diam-diam dilewati — pemanggil membandingkan jumlahnya dan menolak
// permintaannya, supaya tidak ada brief yang terkirim ke lebih sedikit orang
// daripada yang dikira pengirimnya.
func (s *AksesService) Akun(ids []uint) ([]model.User, error) {
	out := make([]model.User, 0, len(ids))
	for _, id := range ids {
		u, err := s.users.FindByID(id)
		if err != nil || u == nil {
			continue
		}
		out = append(out, *u)
	}
	return out, nil
}

// Semua memulangkan seluruh akun untuk pemilih tujuan.
func (s *AksesService) Semua() ([]model.User, error) { return s.users.List() }
