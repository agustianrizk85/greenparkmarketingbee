package handler

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// autoSync holds the background auto-sync state. When enabled, the scheduler
// periodically pulls the Content Plan sheet, applies it (idempotent approve),
// and — when anything new was created — bumps the realtime hub so every open
// dashboard refreshes live, no manual reload needed (same UX as Sales import).
type autoSync struct {
	mu          sync.Mutex
	enabled     bool
	interval    time.Duration
	last        time.Time
	lastErr     string
	lastCreated int
	lastTotal   int
}

func newAutoSync(intervalSec int) *autoSync {
	iv := time.Duration(intervalSec) * time.Second
	if iv <= 0 {
		iv = 10 * time.Minute
	}
	return &autoSync{enabled: false, interval: iv}
}

// minInterval is the floor for auto-sync — one fetch of the public XLSX export
// takes a couple of seconds, so sub-30s polling is wasteful.
const minInterval = 30 * time.Second

// StartAutoSync launches the scheduler goroutine. It checks every 5s whether a
// run is due (enabled && interval elapsed) and runs it.
func (h *ContentPlanHandler) StartAutoSync(ctx context.Context) {
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				a := h.auto
				a.mu.Lock()
				due := a.enabled && time.Since(a.last) >= a.interval
				a.mu.Unlock()
				if !due {
					continue
				}
				h.runAutoSync(ctx)
			}
		}
	}()
}

// runAutoSync performs one fetch+approve and pushes a realtime bump on change.
func (h *ContentPlanHandler) runAutoSync(ctx context.Context) {
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	a := h.auto
	tabs, err := h.fetchTabs(runCtx, "")
	if err == nil {
		ar, e2 := h.svc.Approve(tabs, 0) // createdBy=0 → system/auto
		if e2 != nil {
			err = e2
		} else {
			a.mu.Lock()
			a.lastCreated = ar.Created
			a.lastTotal = ar.Total
			a.mu.Unlock()
			if (ar.Created > 0 || ar.Updated > 0) && h.hub != nil {
				h.hub.Bump() // live refresh on every open dashboard
			}
			log.Printf("content-plan: auto-sync ok (created=%d, updated=%d, total=%d)", ar.Created, ar.Updated, ar.Total)
		}
	}

	a.mu.Lock()
	a.last = time.Now()
	if err != nil {
		a.lastErr = err.Error()
		log.Printf("content-plan: auto-sync error: %v", err)
	} else {
		a.lastErr = ""
	}
	a.mu.Unlock()
}

type autoStatusResp struct {
	Enabled     bool   `json:"enabled"`
	IntervalSec int    `json:"intervalSec"`
	Configured  bool   `json:"configured"`
	LastSync    string `json:"lastSync"`
	LastError   string `json:"lastError"`
	LastCreated int    `json:"lastCreated"`
	LastTotal   int    `json:"lastTotal"`
}

// AutoStatus reports the current auto-sync schedule + last run.
func (h *ContentPlanHandler) AutoStatus(c *gin.Context) {
	a := h.auto
	a.mu.Lock()
	defer a.mu.Unlock()
	last := ""
	if !a.last.IsZero() {
		last = a.last.Format(time.RFC3339)
	}
	c.JSON(http.StatusOK, autoStatusResp{
		Enabled:     a.enabled,
		IntervalSec: int(a.interval / time.Second),
		Configured:  h.defaultSheetID != "", // marketing reads the public export, so always configured
		LastSync:    last,
		LastError:   a.lastErr,
		LastCreated: a.lastCreated,
		LastTotal:   a.lastTotal,
	})
}

type autoSetReq struct {
	Enabled     bool `json:"enabled"`
	IntervalSec int  `json:"intervalSec"`
}

// AutoSet enables/disables auto-sync and sets its interval.
func (h *ContentPlanHandler) AutoSet(c *gin.Context) {
	var req autoSetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	iv := time.Duration(req.IntervalSec) * time.Second
	if iv < minInterval {
		iv = minInterval
	}
	a := h.auto
	a.mu.Lock()
	a.enabled = req.Enabled
	a.interval = iv
	if a.enabled {
		a.last = time.Time{} // run on the next tick
	}
	a.mu.Unlock()
	h.AutoStatus(c)
}
