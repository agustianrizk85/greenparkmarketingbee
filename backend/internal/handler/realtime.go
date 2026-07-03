package handler

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"marketingflow/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// RealtimeHub keeps the set of connected dashboard browsers and pushes a
// data-revision message whenever the backend data changes — giving instant,
// no-refresh updates over a WebSocket. The browser re-fetches on each push.
//
// The revision is bumped by BumpMiddleware after every successful mutating
// request, so any write — by any user — fans out to every open dashboard.
type RealtimeHub struct {
	rev   int64
	mu    sync.Mutex
	conns map[*websocket.Conn]bool
}

// WebSocket keepalive timings. The server pings periodically and expects a pong
// within pongWait; a missed pong trips the read deadline and reaps the (possibly
// half-open) connection, so goroutines don't linger and idle proxies don't drop
// the socket silently.
const (
	wsWriteWait  = 5 * time.Second
	wsPongWait   = 60 * time.Second
	wsPingPeriod = (wsPongWait * 9) / 10
)

func NewRealtimeHub() *RealtimeHub { return &RealtimeHub{conns: map[*websocket.Conn]bool{}} }

func (h *RealtimeHub) revision() int64 { return atomic.LoadInt64(&h.rev) }

func (h *RealtimeHub) bump() { h.broadcast(atomic.AddInt64(&h.rev, 1)) }

// Bump pushes a new revision to every connected dashboard. Exported so
// background jobs (e.g. content-plan auto-sync) can trigger a live refresh
// without going through an HTTP request.
func (h *RealtimeHub) Bump() { h.bump() }

func (h *RealtimeHub) broadcast(rev int64) {
	msg := map[string]int64{"rev": rev}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.conns {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := c.WriteJSON(msg); err != nil {
			delete(h.conns, c)
			_ = c.Close()
		}
	}
}

func (h *RealtimeHub) add(c *websocket.Conn) {
	h.mu.Lock()
	h.conns[c] = true
	h.mu.Unlock()
}

func (h *RealtimeHub) remove(c *websocket.Conn) {
	h.mu.Lock()
	delete(h.conns, c)
	h.mu.Unlock()
	_ = c.Close()
}

func (h *RealtimeHub) sendTo(c *websocket.Conn, rev int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_ = c.WriteJSON(map[string]int64{"rev": rev})
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true }, // same-trust dev/LAN setup
}

// ServeWS upgrades the request to a WebSocket. Browsers cannot send the
// Authorization header on a WS handshake, so the bearer token is passed as a
// query parameter and validated with the token manager.
func (h *RealtimeHub) ServeWS(tm *middleware.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, err := tm.Parse(c.Query("token")); err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		h.add(conn)
		h.sendTo(conn, h.revision()) // sync immediately on connect

		conn.SetReadLimit(512)
		_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(wsPongWait))
		})
		go h.readLoop(conn)
		go h.pingLoop(conn)
	}
}

// readLoop drains inbound frames (there are none of interest) and, crucially,
// lets the read deadline fire when a pong is missed — reaping dead connections.
func (h *RealtimeHub) readLoop(conn *websocket.Conn) {
	defer h.remove(conn)
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// pingLoop sends periodic pings so half-open connections are detected. WriteControl
// is safe to call concurrently with the WriteJSON broadcasts.
func (h *RealtimeHub) pingLoop(conn *websocket.Conn) {
	t := time.NewTicker(wsPingPeriod)
	defer t.Stop()
	for range t.C {
		if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(wsWriteWait)); err != nil {
			h.remove(conn)
			return
		}
	}
}

// BumpMiddleware bumps the revision after every successful mutating request.
func (h *RealtimeHub) BumpMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead &&
			c.Writer.Status() >= 200 && c.Writer.Status() < 300 {
			h.bump()
		}
	}
}
