package handler

import (
	"context"
	"fmt"
	"net/http"

	"marketingflow/internal/gsheets"
	"marketingflow/internal/middleware"
	"marketingflow/internal/service"

	"github.com/gin-gonic/gin"
)

// ContentPlanHandler exposes the Content Plan → work-item sync (preview/approve)
// plus a background auto-sync scheduler that pushes live updates over the
// realtime hub (the "socket" — same pattern as the Sales import).
type ContentPlanHandler struct {
	svc            *service.ContentPlanService
	sync           *gsheets.Client // nil → fall back to public XLSX export
	defaultSheetID string
	hub            *RealtimeHub
	auto           *autoSync
}

func NewContentPlanHandler(svc *service.ContentPlanService, sync *gsheets.Client, defaultSheetID string, hub *RealtimeHub) *ContentPlanHandler {
	return &ContentPlanHandler{svc: svc, sync: sync, defaultSheetID: defaultSheetID, hub: hub, auto: newAutoSync(0)}
}

type syncRequest struct {
	URL string `json:"url"` // optional spreadsheet URL/id; empty → configured default
}

// Source reports the configured spreadsheet id and whether a service account is
// active (so the UI can hint that the sheet must be link-viewable otherwise).
func (h *ContentPlanHandler) Source(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"sheet_id":        h.defaultSheetID,
		"service_account": h.sync != nil,
	})
}

// fetchTabs pulls every tab of the requested (or default) spreadsheet using a
// plain context — shared by the HTTP handlers and the background scheduler.
func (h *ContentPlanHandler) fetchTabs(ctx context.Context, rawURL string) (map[string][][]string, error) {
	id := gsheets.ParseSheetID(rawURL)
	if id == "" {
		id = h.defaultSheetID
	}
	if id == "" {
		return nil, fmt.Errorf("URL/ID spreadsheet kosong — tempel link Google Sheets-nya.")
	}
	if h.sync != nil {
		return h.sync.FetchAll(ctx, id) // service account (private OK)
	}
	return gsheets.FetchPublicXLSX(ctx, id) // public link, no credentials
}

// fetch is the gin wrapper: it reads the optional body, fetches, and writes the
// error response itself, returning ok=false on failure.
func (h *ContentPlanHandler) fetch(c *gin.Context) (map[string][][]string, bool) {
	var body syncRequest
	_ = c.ShouldBindJSON(&body) // body is optional
	data, err := h.fetchTabs(c.Request.Context(), body.URL)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "gagal ambil spreadsheet: " + err.Error()})
		return nil, false
	}
	return data, true
}

// Preview returns a dry-run of the sync (counts + per-item new/existing).
func (h *ContentPlanHandler) Preview(c *gin.Context) {
	data, ok := h.fetch(c)
	if !ok {
		return
	}
	res, err := h.svc.Preview(data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// Approve applies the sync, creating work items for new planned content.
func (h *ContentPlanHandler) Approve(c *gin.Context) {
	data, ok := h.fetch(c)
	if !ok {
		return
	}
	res, err := h.svc.Approve(data, middleware.CurrentUserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}
