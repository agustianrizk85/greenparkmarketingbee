package handler

import (
	"net/http"

	"marketingflow/internal/service"

	"github.com/gin-gonic/gin"
)

type DashboardHandler struct {
	dashboard *service.DashboardService
}

func NewDashboardHandler(dashboard *service.DashboardService) *DashboardHandler {
	return &DashboardHandler{dashboard: dashboard}
}

// EarlyWarnings returns the early-warning feed across all work items.
func (h *DashboardHandler) EarlyWarnings(c *gin.Context) {
	warnings, err := h.dashboard.EarlyWarnings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"warnings": warnings, "count": len(warnings)})
}
