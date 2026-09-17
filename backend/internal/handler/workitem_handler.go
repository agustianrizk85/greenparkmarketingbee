package handler

import (
	"errors"
	"net/http"
	"strconv"

	"marketingflow/internal/dto"
	"marketingflow/internal/middleware"
	"marketingflow/internal/model"
	"marketingflow/internal/repository"
	"marketingflow/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AksesPosisi mencari POSISI pemakai (Copywriter, Design Grafis, …) dari
// akunnya. Posisi tidak ikut di token, jadi ia dibaca dari basis data — lewat id
// untuk akun asli marketing, lewat email untuk akun SSO yang id-nya 0.
type AksesPosisi interface {
	Posisi(id uint, email string) string
	Akun(ids []uint) ([]model.User, error)
	Semua() ([]model.User, error)
}

type WorkItemHandler struct {
	items *service.WorkItemService
	docs  *service.DocumentService
	akses AksesPosisi
}

func NewWorkItemHandler(items *service.WorkItemService, docs *service.DocumentService, akses AksesPosisi) *WorkItemHandler {
	return &WorkItemHandler{items: items, docs: docs, akses: akses}
}

// Reset deletes ALL work items, steps and documents (keeping accounts). Gated to
// Kepala Departemen. Used by the "Hapus Semua Data" action.
func (h *WorkItemHandler) Reset(c *gin.Context) {
	counts, err := h.items.DeleteAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.docs.PurgeAll(); err != nil {
		// Rows are already gone; a file-cleanup failure is non-fatal — report it.
		c.JSON(http.StatusOK, gin.H{"deleted": counts, "warning": "file lampiran tidak terhapus seluruhnya: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": counts})
}

func (h *WorkItemHandler) Create(c *gin.Context) {
	var req dto.CreateWorkItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := h.items.Create(req, middleware.CurrentUserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, item)
}

// UpdateBrief menyimpan catatan kartu (isi brief).
func (h *WorkItemHandler) UpdateBrief(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak sah"})
		return
	}
	var req dto.UpdateBriefRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err = h.items.UpdateBrief(uint(id), req.Brief, middleware.CurrentUserID(c), middleware.CurrentRole(c))
	switch {
	case errors.Is(err, service.ErrTidakBerhak):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "konten tidak ditemukan"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusOK, gin.H{"brief": req.Brief})
	}
}

// Pindahkan memindahkan kartu ke kolom lain.
func (h *WorkItemHandler) Pindahkan(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak sah"})
		return
	}
	var req dto.UpdateStageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err = h.items.Pindahkan(uint(id), req.Stage, middleware.CurrentUserID(c), middleware.CurrentRole(c),
		h.akses.Posisi(middleware.CurrentUserID(c), middleware.CurrentEmail(c)))
	switch {
	case errors.Is(err, service.ErrStageTidakDikenal):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrTidakBerhak), errors.Is(err, service.ErrReviewRole):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "konten tidak ditemukan"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusOK, gin.H{"stage": req.Stage})
	}
}

// Delete menghapus satu konten beserta langkah, komentar, dan lampirannya.
func (h *WorkItemHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak sah"})
		return
	}
	counts, berkas, err := h.items.Delete(uint(id), middleware.CurrentUserID(c), middleware.CurrentRole(c))
	if errors.Is(err, service.ErrTidakBerhak) {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "konten tidak ditemukan"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Barisnya sudah hilang; gagal menghapus berkas bukan alasan menggagalkan
	// permintaan, tapi tetap dilaporkan supaya tidak diam-diam menumpuk.
	if sisa := h.docs.PurgeFiles(berkas); sisa != nil {
		c.JSON(http.StatusOK, gin.H{"deleted": counts, "warning": "file lampiran tidak terhapus seluruhnya: " + sisa.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": counts})
}

// PreBrief membuka pekerjaan baru untuk beberapa orang sekaligus: satu konten
// per penerima, langsung muncul di papan masing-masing.
//
// Haknya di Kepala Departemen dan Copywriter — merekalah yang menulis brief di
// alur kerja ini. Dinilai di sini (bukan lewat RequireRole di rute) karena
// syaratnya bukan cuma peran, tapi juga POSISI.
func (h *WorkItemHandler) PreBrief(c *gin.Context) {
	posisi := h.akses.Posisi(middleware.CurrentUserID(c), middleware.CurrentEmail(c))
	if middleware.CurrentRole(c) != model.RoleKadep && posisi != service.OwnerCopywriter {
		c.JSON(http.StatusForbidden, gin.H{"error": "hanya Kepala Departemen dan Copywriter yang dapat membuat pre-brief"})
		return
	}
	var req dto.PreBriefRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ids := make([]uint, 0, len(req.Assignees))
	for _, t := range req.Assignees {
		ids = append(ids, t.UserID)
	}
	akun, err := h.akses.Akun(ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(akun) != len(ids) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ada akun tujuan yang tidak ditemukan"})
		return
	}
	// Dipasangkan lewat peta, bukan mengandalkan urutan balikan basis data —
	// brief yang tertukar orang lebih buruk daripada permintaan yang gagal.
	akunPer := make(map[uint]model.User, len(akun))
	for _, u := range akun {
		akunPer[u.ID] = u
	}
	penerima := make([]service.PenerimaBrief, 0, len(req.Assignees))
	for _, t := range req.Assignees {
		penerima = append(penerima, service.PenerimaBrief{User: akunPer[t.UserID], Brief: t.Brief})
	}
	items, err := h.items.CreatePreBrief(req, middleware.CurrentUserID(c), penerima)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"items": items})
}

// Akun memulangkan daftar akun Marketing untuk pemilih tujuan Pre-Brief.
// Hanya nama, posisi, dan peran — tidak ada surel maupun kata sandi.
func (h *WorkItemHandler) Akun(c *gin.Context) {
	users, err := h.akses.Semua()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]gin.H, 0, len(users))
	for _, u := range users {
		out = append(out, gin.H{"id": u.ID, "name": u.Name, "position": u.Position, "role": u.Role})
	}
	c.JSON(http.StatusOK, gin.H{"users": out})
}

func (h *WorkItemHandler) List(c *gin.Context) {
	items, err := h.items.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *WorkItemHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	item, err := h.items.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "work item not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *WorkItemHandler) Progress(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	progress, err := h.items.Progress(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, progress)
}

// parseID extracts and validates the :id route parameter.
func parseID(c *gin.Context) (uint, bool) {
	raw := c.Param("id")
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return 0, false
	}
	return uint(n), true
}
