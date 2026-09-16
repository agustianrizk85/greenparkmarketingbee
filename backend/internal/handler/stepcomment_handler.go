package handler

// KOMENTAR LANGKAH — GET/POST /api/steps/:id/comments
//
// Percakapan antar tim pada satu langkah alur konten (lihat model.StepComment).
// Siapa pun yang boleh membuka modul ini boleh ikut bicara, termasuk pembaca:
// menahan komentar dari orang yang bisa membacanya tidak melindungi apa pun,
// hanya memindahkan percakapannya ke WhatsApp.

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"marketingflow/internal/middleware"
	"marketingflow/internal/model"
)

// komentarMaks membatasi panjang satu komentar. Bukan soal penyimpanan: kotak
// komentar yang menerima naskah sepanjang apa pun akan dipakai menempel brief
// utuh, dan utas jadi tidak terbaca.
const komentarMaks = 2000

type StepCommentHandler struct{ db *gorm.DB }

func NewStepCommentHandler(db *gorm.DB) *StepCommentHandler { return &StepCommentHandler{db: db} }

func stepIDParam(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id langkah tidak sah"})
		return 0, false
	}
	return uint(id), true
}

// List mengembalikan seluruh komentar satu langkah, terlama dulu (urutan baca).
func (h *StepCommentHandler) List(c *gin.Context) {
	id, ok := stepIDParam(c)
	if !ok {
		return
	}
	out := []model.StepComment{}
	if err := h.db.Where("step_id = ?", id).Order("created_at").Find(&out).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"comments": out})
}

// ListByItem mengembalikan seluruh komentar SEMUA langkah dalam satu konten.
//
// Satu permintaan untuk seluruh pohon, bukan satu per langkah: sebuah konten
// punya sepuluh sampai dua belas langkah, dan meminta daftarnya satu per satu
// berarti selusin permintaan tiap kali kartu dibuka - padahal jawabannya hampir
// selalu kosong. Dengan satu panggilan, tiap baris langkah juga bisa langsung
// menyebut jumlah komentarnya tanpa dibuka.
func (h *StepCommentHandler) ListByItem(c *gin.Context) {
	id, ok := stepIDParam(c) // di rute ini :id adalah id KONTEN
	if !ok {
		return
	}
	out := []model.StepComment{}
	langkah := h.db.Model(&model.WorkStep{}).Select("id").Where("work_item_id = ?", id)
	if err := h.db.Where("step_id IN (?)", langkah).Order("created_at").Find(&out).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"comments": out})
}

// Create menulis satu komentar atas nama pemanggil.
func (h *StepCommentHandler) Create(c *gin.Context) {
	id, ok := stepIDParam(c)
	if !ok {
		return
	}
	// Langkahnya harus ada: komentar pada langkah yang tidak ada tidak akan
	// pernah terbaca siapa pun, dan diam-diam tersimpan lebih buruk daripada
	// ditolak terang-terangan.
	var n int64
	if err := h.db.Model(&model.WorkStep{}).Where("id = ?", id).Count(&n).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "langkah tidak ditemukan"})
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body JSON tidak valid: " + err.Error()})
		return
	}
	teks := strings.TrimSpace(body.Text)
	if teks == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "komentar kosong"})
		return
	}
	if len(teks) > komentarMaks {
		teks = teks[:komentarMaks]
	}

	author := middleware.CurrentEmail(c)
	k := model.StepComment{StepID: id, Author: author, Text: teks}
	// Nama & posisi diambil dari akun marketing bila ada; kalau tidak (direksi /
	// akun SSO lintas divisi), identitasnya sendiri yang ditampilkan.
	var u model.User
	if author != "" && h.db.Where("email = ?", author).First(&u).Error == nil {
		k.AuthorName = u.Name
		k.AuthorRole = u.Position
	}
	if k.AuthorName == "" {
		k.AuthorName = author
	}
	if err := h.db.Create(&k).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, k)
}
