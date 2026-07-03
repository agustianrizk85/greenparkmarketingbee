package handler

import (
	"errors"
	"net/http"
	"strconv"

	"marketingflow/internal/dto"
	"marketingflow/internal/middleware"
	"marketingflow/internal/repository"
	"marketingflow/internal/service"

	"github.com/gin-gonic/gin"
)

type WorkItemHandler struct {
	items *service.WorkItemService
	docs  *service.DocumentService
}

func NewWorkItemHandler(items *service.WorkItemService, docs *service.DocumentService) *WorkItemHandler {
	return &WorkItemHandler{items: items, docs: docs}
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
