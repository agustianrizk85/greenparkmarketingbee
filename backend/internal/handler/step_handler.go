package handler

import (
	"errors"
	"net/http"

	"marketingflow/internal/dto"
	"marketingflow/internal/middleware"
	"marketingflow/internal/repository"
	"marketingflow/internal/service"

	"github.com/gin-gonic/gin"
)

type StepHandler struct {
	steps *service.StepService
	docs  *service.DocumentService
}

func NewStepHandler(steps *service.StepService, docs *service.DocumentService) *StepHandler {
	return &StepHandler{steps: steps, docs: docs}
}

func (h *StepHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	step, err := h.steps.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "step not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, step)
}

// Mine returns the steps owned by the position given in ?position= (defaults to
// the caller passing their own). Powers the "Tugas Saya" board + mobile view.
func (h *StepHandler) Mine(c *gin.Context) {
	steps, err := h.steps.Mine(c.Query("position"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"steps": steps, "count": len(steps)})
}

func (h *StepHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.UpdateStepRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	step, err := h.steps.Update(id, req, middleware.CurrentUserID(c), middleware.CurrentRole(c))
	switch {
	case errors.Is(err, repository.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "step not found"})
	case errors.Is(err, service.ErrBudgetRequired), errors.Is(err, service.ErrApprovalRole):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusOK, step)
	}
}

// UploadDocument handles multipart upload of a file attached to a step.
func (h *StepHandler) UploadDocument(c *gin.Context) {
	stepID, ok := parseID(c)
	if !ok {
		return
	}
	step, err := h.steps.Get(stepID)
	if errors.Is(err, repository.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "step not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	docType := c.PostForm("doc_type")
	if docType == "" {
		docType = step.Code
	}

	doc, err := h.docs.Upload(fh, service.UploadParams{
		WorkItemID: step.WorkItemID,
		WorkStepID: &step.ID,
		DocType:    docType,
		UploadedBy: middleware.CurrentUserID(c),
	}, c.SaveUploadedFile)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, doc)
}

// DownloadDocument streams a stored file back to the client.
func (h *StepHandler) DownloadDocument(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	doc, err := h.docs.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.FileAttachment(doc.Path, doc.OriginalName)
}
