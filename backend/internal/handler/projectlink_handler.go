package handler

// TAUTAN PROYEK — GET /api/project-links, PUT /api/project-links/:name
//
// Menautkan proyek konten ke proyek Perencanaan (lihat model.ProjectLink).
// Menautkan cukup satu kolom, tidak merusak apa pun, dan bisa dibalik - jadi
// boleh dilakukan siapa pun kecuali pembaca (viewer).

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"marketingflow/internal/middleware"
	"marketingflow/internal/model"
)

type ProjectLinkHandler struct {
	db *gorm.DB
}

func NewProjectLinkHandler(db *gorm.DB) *ProjectLinkHandler { return &ProjectLinkHandler{db: db} }

// List mengembalikan semua tautan; daftarnya kecil (satu baris per proyek).
func (h *ProjectLinkHandler) List(c *gin.Context) {
	links := []model.ProjectLink{}
	if err := h.db.Order("project_name").Find(&links).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"links": links})
}

// Put menyimpan (upsert) tautan satu proyek. Id kosong = melepaskan tautan.
func (h *ProjectLinkHandler) Put(c *gin.Context) {
	if middleware.CurrentRole(c) == model.RoleViewer {
		c.JSON(http.StatusForbidden, gin.H{"error": "pembaca tidak bisa menautkan proyek"})
		return
	}
	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nama proyek kosong"})
		return
	}
	var body struct {
		PerencanaanProjectID string `json:"perencanaan_project_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body JSON tidak valid: " + err.Error()})
		return
	}
	link := model.ProjectLink{
		ProjectName:          name,
		PerencanaanProjectID: strings.TrimSpace(body.PerencanaanProjectID),
		UpdatedBy:            middleware.CurrentUserID(c),
		UpdatedAt:            time.Now(),
	}
	// Save = upsert menurut primary key (nama proyek).
	if err := h.db.Save(&link).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, link)
}
