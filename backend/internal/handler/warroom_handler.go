package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"marketingflow/internal/middleware"
	"marketingflow/internal/model"
	"marketingflow/internal/repository"
	"marketingflow/internal/service"

	"github.com/gin-gonic/gin"
)

// WAR ROOM — satu muatan angka untuk layar, plus catatan perintah yang lahir
// darinya.
//
// Yang TIDAK ada di sini: narasi. Kalimat briefing ditulis AI di frontend lewat
// gateway AI, dan hanya setelah lolos pemeriksaan kode. Backend ini sengaja
// tidak pernah memanggil AI — dengan begitu angka di layar tidak pernah
// bergantung pada layanan AI yang sedang sibuk atau mati.
type WarRoomHandler struct {
	svc  *service.WarRoomService
	repo *repository.WarroomRepository
}

func NewWarRoomHandler(svc *service.WarRoomService, repo *repository.WarroomRepository) *WarRoomHandler {
	return &WarRoomHandler{svc: svc, repo: repo}
}

// bearer mengambil token pemanggil untuk diteruskan ke metaapi dan Sales.
func bearer(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}

// Muatan melayani GET /api/warroom?proyek_id=
//
// Selalu 200 selama pemanggilnya sah: sumber yang mati muncul sebagai dimensi
// abu beserta alasannya di celah_data, bukan sebagai layar galat. War room yang
// padam karena satu layanan ngambek akan ditinggalkan orang justru pada hari
// ketika ia paling dibutuhkan.
func (h *WarRoomHandler) Muatan(c *gin.Context) {
	wr := h.svc.Susun(c.Request.Context(), bearer(c), service.WRLingkup{ProyekID: c.Query("proyek_id")})
	c.JSON(http.StatusOK, wr)
}

/* ---- catatan keputusan ---------------------------------------------------- */

type keputusanReq struct {
	Sev               string   `json:"sev"`
	Judul             string   `json:"judul"`
	Tindakan          string   `json:"tindakan"`
	TargetTipe        string   `json:"targetTipe"`
	TargetID          string   `json:"targetId"`
	TargetNama        string   `json:"targetNama"`
	Alasan            string   `json:"alasan"`
	Bukti             []string `json:"bukti"`
	UangDipertaruhkan string   `json:"uangDipertaruhkan"`
	DampakDiharapkan  string   `json:"dampakDiharapkan"`
	PIC               string   `json:"pic"`
	Tenggat           string   `json:"tenggat"`
	CekPada           string   `json:"cekPada"` // YYYY-MM-DD
	MetrikVerifikasi  string   `json:"metrikVerifikasi"`
}

// Catat melayani POST /api/warroom/keputusan — usulan yang DITERIMA CEO.
//
// Syaratnya ketat di tiga hal yang membuat perintah bisa diverifikasi nanti:
// tindakan harus dari daftar yang dikenal, tanggal cek wajib ada, dan metrik
// verifikasi wajib diisi. Perintah tanpa ketiganya akan jadi catatan yang tidak
// pernah bisa dinilai berhasil atau gagal — dan itu sama saja dengan tidak ada.
func (h *WarRoomHandler) Catat(c *gin.Context) {
	var in keputusanReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "isi permintaan tidak terbaca"})
		return
	}
	in.Judul = strings.TrimSpace(in.Judul)
	in.Tindakan = strings.TrimSpace(strings.ToLower(in.Tindakan))
	if in.Judul == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "judul keputusan wajib diisi"})
		return
	}
	if !model.TindakanSah(in.Tindakan) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tindakan tidak dikenal: " + in.Tindakan})
		return
	}
	if strings.TrimSpace(in.MetrikVerifikasi) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "metrik verifikasi wajib diisi — tanpa itu hasilnya tidak bisa dinilai"})
		return
	}
	cek, err := time.Parse("2006-01-02", strings.TrimSpace(in.CekPada))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tanggal cek wajib berbentuk YYYY-MM-DD"})
		return
	}
	bukti, _ := json.Marshal(in.Bukti)

	k := model.WarroomKeputusan{
		Sev: in.Sev, Judul: in.Judul, Tindakan: in.Tindakan,
		TargetTipe: in.TargetTipe, TargetID: in.TargetID, TargetNama: in.TargetNama,
		Alasan: in.Alasan, Bukti: string(bukti),
		UangDipertaruhkan: in.UangDipertaruhkan, DampakDiharapkan: in.DampakDiharapkan,
		PIC: in.PIC, Tenggat: in.Tenggat, MetrikVerifikasi: in.MetrikVerifikasi,
		CekPada: cek, Status: model.KeputusanTerbuka,
		DibuatOleh: middleware.CurrentEmail(c), DibuatPada: time.Now(),
	}
	if err := h.repo.Catat(&k); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, k)
}

// Daftar melayani GET /api/warroom/keputusan?status=&jatuh_tempo=1&limit=
func (h *WarRoomHandler) Daftar(c *gin.Context) {
	batas, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if batas <= 0 || batas > 200 {
		batas = 50
	}
	var (
		out []model.WarroomKeputusan
		err error
	)
	if c.Query("jatuh_tempo") == "1" {
		// Bahan KEPUTUSAN_TERAKHIR untuk AI: yang masih terbuka + yang baru
		// selesai, supaya briefing bisa melaporkan hasilnya tanpa mengarang.
		out, err = h.repo.JatuhTempo(time.Now(), batas)
	} else {
		out, err = h.repo.Daftar(strings.TrimSpace(c.Query("status")), batas)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// Verifikasi melayani PATCH /api/warroom/keputusan/:id — menutup satu perintah.
func (h *WarRoomHandler) Verifikasi(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak sah"})
		return
	}
	var in struct {
		Status string `json:"status"`
		Bukti  string `json:"bukti"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "isi permintaan tidak terbaca"})
		return
	}
	in.Status = strings.TrimSpace(strings.ToLower(in.Status))
	if !model.StatusKeputusanSah(in.Status) || in.Status == model.KeputusanTerbuka {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status hasil harus berhasil, belum jelas, atau gagal"})
		return
	}
	if _, err := h.repo.Ambil(uint(id)); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "keputusan tidak ditemukan"})
		return
	}
	if err := h.repo.Verifikasi(uint(id), in.Status, in.Bukti, middleware.CurrentEmail(c), time.Now()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	k, _ := h.repo.Ambil(uint(id))
	c.JSON(http.StatusOK, k)
}

// Hapus melayani DELETE /api/warroom/keputusan/:id (salah catat).
func (h *WarRoomHandler) Hapus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak sah"})
		return
	}
	if err := h.repo.Hapus(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
