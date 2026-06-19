package handler

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// QualifiedDemandHandler serves the Marketing "Qualified Demand Control Tower"
// payload (channel performance, MQL scoring, MQL→SAL handover, demand/readiness
// and digital-asset health). The data lives in an editable JSON seed file and is
// re-read on every request, so updating the file is reflected without a restart.
type QualifiedDemandHandler struct {
	path string
}

func NewQualifiedDemandHandler(path string) *QualifiedDemandHandler {
	if path == "" {
		path = "data/qualified-demand.json"
	}
	return &QualifiedDemandHandler{path: path}
}

// Get returns the qualified-demand dataset.
func (h *QualifiedDemandHandler) Get(c *gin.Context) {
	b, err := os.ReadFile(h.path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "data qualified-demand belum tersedia: " + err.Error()})
		return
	}
	var payload any
	if err := json.Unmarshal(b, &payload); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "data qualified-demand rusak: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, payload)
}
