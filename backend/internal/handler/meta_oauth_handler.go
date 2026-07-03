package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"marketingflow/internal/config"
	"marketingflow/internal/middleware"
	"marketingflow/internal/model"
	"marketingflow/internal/repository"

	"github.com/gin-gonic/gin"
)

// MetaOAuthHandler runs the Facebook Login (OAuth) flow and manages the set of
// connected Meta accounts. App credentials are dynamic (stored in the DB, set
// from the UI) so the team can register one Meta App and connect several
// accounts, switching which one is active. The env values only seed defaults.
type MetaOAuthHandler struct {
	repo   *repository.MetaRepository
	tokens *middleware.TokenManager
	cfg    *config.Config
	http   *http.Client

	mu     sync.Mutex
	states map[string]oauthState // CSRF state → initiating user, short-lived
}

type oauthState struct {
	userID uint
	exp    time.Time
}

func NewMetaOAuthHandler(repo *repository.MetaRepository, tokens *middleware.TokenManager, cfg *config.Config) *MetaOAuthHandler {
	return &MetaOAuthHandler{
		repo:   repo,
		tokens: tokens,
		cfg:    cfg,
		http:   &http.Client{Timeout: 25 * time.Second},
		states: map[string]oauthState{},
	}
}

// effectiveConfig merges the stored config with env-seeded defaults so a field
// left blank in the DB still resolves to a sane value.
func (h *MetaOAuthHandler) effectiveConfig() (*model.MetaAppConfig, error) {
	c, err := h.repo.AppConfig()
	if err != nil {
		return nil, err
	}
	if c.AppID == "" {
		c.AppID = h.cfg.MetaAppID
	}
	if c.AppSecret == "" {
		c.AppSecret = h.cfg.MetaAppSecret
	}
	if c.RedirectURI == "" {
		c.RedirectURI = h.cfg.MetaRedirectURI
	}
	if c.APIVersion == "" {
		c.APIVersion = h.cfg.MetaAPIVersion
	}
	if c.Scopes == "" {
		c.Scopes = h.cfg.MetaScopes
	}
	return c, nil
}

func (c *MetaOAuthHandler) configuredStatus(cfg *model.MetaAppConfig) bool {
	return cfg.AppID != "" && cfg.AppSecret != ""
}

// Config returns the current OAuth app config (secret masked) plus a count of
// connected accounts so the UI can show readiness.
func (h *MetaOAuthHandler) Config(c *gin.Context) {
	cfg, err := h.effectiveConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	count, _ := h.repo.CountConnections()
	c.JSON(http.StatusOK, gin.H{
		"app_id":       cfg.AppID,
		"redirect_uri": cfg.RedirectURI,
		"api_version":  cfg.APIVersion,
		"scopes":       cfg.Scopes,
		"has_secret":   cfg.AppSecret != "",
		"configured":   h.configuredStatus(cfg),
		"connections":  count,
	})
}

type saveConfigRequest struct {
	AppID       *string `json:"app_id"`
	AppSecret   *string `json:"app_secret"`
	RedirectURI *string `json:"redirect_uri"`
	APIVersion  *string `json:"api_version"`
	Scopes      *string `json:"scopes"`
}

// SaveConfig upserts the OAuth app credentials. A blank/omitted secret keeps the
// stored one so the UI never needs to re-enter it.
func (h *MetaOAuthHandler) SaveConfig(c *gin.Context) {
	var req saveConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg, err := h.repo.AppConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if req.AppID != nil {
		cfg.AppID = strings.TrimSpace(*req.AppID)
	}
	if req.AppSecret != nil && strings.TrimSpace(*req.AppSecret) != "" {
		cfg.AppSecret = strings.TrimSpace(*req.AppSecret)
	}
	if req.RedirectURI != nil {
		cfg.RedirectURI = strings.TrimSpace(*req.RedirectURI)
	}
	if req.APIVersion != nil {
		cfg.APIVersion = strings.TrimSpace(*req.APIVersion)
	}
	if req.Scopes != nil {
		cfg.Scopes = strings.TrimSpace(*req.Scopes)
	}
	if err := h.repo.SaveAppConfig(cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.Config(c)
}

// newState mints a one-time CSRF state bound to the initiating user.
func (h *MetaOAuthHandler) newState(userID uint) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	s := hex.EncodeToString(b)
	h.mu.Lock()
	defer h.mu.Unlock()
	// Opportunistically drop expired entries.
	now := time.Now()
	for k, v := range h.states {
		if now.After(v.exp) {
			delete(h.states, k)
		}
	}
	h.states[s] = oauthState{userID: userID, exp: now.Add(10 * time.Minute)}
	return s
}

func (h *MetaOAuthHandler) consumeState(s string) (uint, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	st, ok := h.states[s]
	if !ok {
		return 0, false
	}
	delete(h.states, s)
	if time.Now().After(st.exp) {
		return 0, false
	}
	return st.userID, true
}

// Login starts the OAuth flow. It is a top-level navigation (popup), so it
// authenticates via ?token= (browsers can't set headers on a popup) and 302s to
// Facebook's consent dialog.
func (h *MetaOAuthHandler) Login(c *gin.Context) {
	claims, err := h.tokens.Parse(c.Query("token"))
	if err != nil {
		c.Data(http.StatusUnauthorized, "text/html; charset=utf-8", popupResult(false, "Sesi tidak valid. Login ulang lalu coba lagi."))
		return
	}
	cfg, err := h.effectiveConfig()
	if err != nil {
		c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", popupResult(false, err.Error()))
		return
	}
	if !h.configuredStatus(cfg) {
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", popupResult(false, "Meta App belum dikonfigurasi (App ID & App Secret)."))
		return
	}
	state := h.newState(claims.UserID)
	auth, _ := url.Parse("https://www.facebook.com/" + cfg.APIVersion + "/dialog/oauth")
	q := auth.Query()
	q.Set("client_id", cfg.AppID)
	q.Set("redirect_uri", cfg.RedirectURI)
	q.Set("state", state)
	q.Set("response_type", "code")
	q.Set("scope", strings.ReplaceAll(cfg.Scopes, " ", ""))
	auth.RawQuery = q.Encode()
	c.Redirect(http.StatusFound, auth.String())
}

// Callback completes the flow: validates state, exchanges the code for a
// long-lived token, identifies the account, and stores (or refreshes) the
// connection. It returns a tiny HTML page that notifies the opener and closes.
func (h *MetaOAuthHandler) Callback(c *gin.Context) {
	if e := c.Query("error"); e != "" {
		desc := c.Query("error_description")
		if desc == "" {
			desc = e
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", popupResult(false, "Dibatalkan / ditolak: "+desc))
		return
	}
	userID, ok := h.consumeState(c.Query("state"))
	if !ok {
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", popupResult(false, "State tidak valid atau kedaluwarsa."))
		return
	}
	code := c.Query("code")
	if code == "" {
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", popupResult(false, "Tidak ada authorization code."))
		return
	}
	cfg, err := h.effectiveConfig()
	if err != nil || !h.configuredStatus(cfg) {
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", popupResult(false, "Konfigurasi Meta App tidak lengkap."))
		return
	}

	// 1. code → short-lived token
	short, err := h.exchange(cfg, map[string]string{
		"client_id":     cfg.AppID,
		"client_secret": cfg.AppSecret,
		"redirect_uri":  cfg.RedirectURI,
		"code":          code,
	})
	if err != nil {
		c.Data(http.StatusBadGateway, "text/html; charset=utf-8", popupResult(false, "Tukar token gagal: "+err.Error()))
		return
	}
	// 2. short → long-lived token (~60 days)
	token := short.AccessToken
	expiresIn := short.ExpiresIn
	if long, err := h.exchange(cfg, map[string]string{
		"grant_type":        "fb_exchange_token",
		"client_id":         cfg.AppID,
		"client_secret":     cfg.AppSecret,
		"fb_exchange_token": short.AccessToken,
	}); err == nil && long.AccessToken != "" {
		token = long.AccessToken
		expiresIn = long.ExpiresIn
	}

	// 3. identify the account behind the token
	me, err := h.graphMe(cfg, token)
	if err != nil {
		c.Data(http.StatusBadGateway, "text/html; charset=utf-8", popupResult(false, "Gagal membaca akun Meta: "+err.Error()))
		return
	}

	var expPtr *time.Time
	if expiresIn > 0 {
		t := time.Now().Add(time.Duration(expiresIn) * time.Second)
		expPtr = &t
	}

	// Upsert by Meta user id so re-connecting refreshes the token in place.
	conn, ferr := h.repo.FindConnectionByMetaUserID(me.ID)
	if ferr == nil && conn != nil {
		conn.AccessToken = token
		conn.TokenExpiresAt = expPtr
		conn.MetaUserName = me.Name
		conn.Scopes = cfg.Scopes
		if conn.Label == "" {
			conn.Label = me.Name
		}
		if err := h.repo.SaveConnection(conn); err != nil {
			c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", popupResult(false, err.Error()))
			return
		}
	} else {
		conn = &model.MetaConnection{
			Label:          me.Name,
			MetaUserID:     me.ID,
			MetaUserName:   me.Name,
			AccessToken:    token,
			TokenExpiresAt: expPtr,
			Scopes:         cfg.Scopes,
			CreatedBy:      userID,
		}
		if err := h.repo.CreateConnection(conn); err != nil {
			c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", popupResult(false, err.Error()))
			return
		}
	}
	// The freshly connected account becomes the active one.
	if err := h.repo.SetActive(conn.ID); err != nil {
		log.Printf("meta oauth: gagal set akun aktif (id=%d): %v", conn.ID, err)
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", popupResult(true, me.Name))
}

type connectManualRequest struct {
	AccessToken string `json:"access_token"`
	Label       string `json:"label"`
}

// ConnectManual stores a manually-supplied access token (e.g. a System User
// token from Business Settings, or a long-lived token from the Graph API
// Explorer) as a connected account — NO OAuth dialog, NO redirect URI, NO
// "App Not Active" wall. The token is validated against /me, upserted by Meta
// user id, and made the active account. Long-lived/permanent tokens carry no
// expiry, so token_expires_at stays null.
func (h *MetaOAuthHandler) ConnectManual(c *gin.Context) {
	var req connectManualRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	token := strings.TrimSpace(req.AccessToken)
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Access token wajib diisi."})
		return
	}
	cfg, err := h.effectiveConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	me, err := h.graphMe(cfg, token)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Token tidak valid / gagal membaca akun Meta: " + err.Error()})
		return
	}
	userID := middleware.CurrentUserID(c)
	label := strings.TrimSpace(req.Label)

	// Upsert by Meta user id so re-pasting a fresh token refreshes in place.
	conn, ferr := h.repo.FindConnectionByMetaUserID(me.ID)
	if ferr == nil && conn != nil {
		conn.AccessToken = token
		conn.TokenExpiresAt = nil
		conn.MetaUserName = me.Name
		if label != "" {
			conn.Label = label
		} else if conn.Label == "" {
			conn.Label = me.Name
		}
		if err := h.repo.SaveConnection(conn); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		if label == "" {
			label = me.Name
		}
		conn = &model.MetaConnection{
			Label:        label,
			MetaUserID:   me.ID,
			MetaUserName: me.Name,
			AccessToken:  token,
			Scopes:       cfg.Scopes,
			CreatedBy:    userID,
		}
		if err := h.repo.CreateConnection(conn); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if err := h.repo.SetActive(conn.ID); err != nil {
		log.Printf("meta connect manual: gagal set akun aktif (id=%d): %v", conn.ID, err)
	}
	h.ListConnections(c)
}

// ListConnections returns all connected accounts (tokens never serialised).
func (h *MetaOAuthHandler) ListConnections(c *gin.Context) {
	conns, err := h.repo.ListConnections()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"connections": conns, "count": len(conns)})
}

// Activate switches which connected account the Graph proxy uses.
func (h *MetaOAuthHandler) Activate(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.repo.SetActive(id); err != nil {
		status := http.StatusInternalServerError
		if err == repository.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	h.ListConnections(c)
}

type updateConnectionRequest struct {
	Label       *string `json:"label"`
	AdAccountID *string `json:"ad_account_id"`
	BusinessID  *string `json:"business_id"`
}

// UpdateConnection edits per-account display/pinning (label, pinned ad account,
// business id) — useful when one login sees several ad accounts.
func (h *MetaOAuthHandler) UpdateConnection(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req updateConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	conn, err := h.repo.FindConnection(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
		return
	}
	if req.Label != nil {
		conn.Label = strings.TrimSpace(*req.Label)
	}
	if req.AdAccountID != nil {
		conn.AdAccountID = strings.TrimPrefix(strings.TrimSpace(*req.AdAccountID), "act_")
	}
	if req.BusinessID != nil {
		conn.BusinessID = strings.TrimSpace(*req.BusinessID)
	}
	if err := h.repo.SaveConnection(conn); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.ListConnections(c)
}

// Disconnect removes a connected account.
func (h *MetaOAuthHandler) Disconnect(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.repo.DeleteConnection(id); err != nil {
		status := http.StatusInternalServerError
		if err == repository.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	h.ListConnections(c)
}

// --- Graph helpers ---

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

type graphError struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

type graphMe struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (h *MetaOAuthHandler) exchange(cfg *model.MetaAppConfig, params map[string]string) (*tokenResponse, error) {
	u, _ := url.Parse("https://graph.facebook.com/" + cfg.APIVersion + "/oauth/access_token")
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	res, err := h.http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := readAll(res.Body)
	if res.StatusCode >= 400 {
		var ge graphError
		_ = json.Unmarshal(body, &ge)
		if ge.Error.Message != "" {
			return nil, errString(ge.Error.Message)
		}
		return nil, errString("graph error " + res.Status)
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, err
	}
	return &tr, nil
}

func (h *MetaOAuthHandler) graphMe(cfg *model.MetaAppConfig, token string) (*graphMe, error) {
	u, _ := url.Parse("https://graph.facebook.com/" + cfg.APIVersion + "/me")
	q := u.Query()
	q.Set("fields", "id,name")
	q.Set("access_token", token)
	u.RawQuery = q.Encode()
	res, err := h.http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := readAll(res.Body)
	if res.StatusCode >= 400 {
		var ge graphError
		_ = json.Unmarshal(body, &ge)
		if ge.Error.Message != "" {
			return nil, errString(ge.Error.Message)
		}
		return nil, errString("graph error " + res.Status)
	}
	var me graphMe
	if err := json.Unmarshal(body, &me); err != nil {
		return nil, err
	}
	return &me, nil
}

// popupResult renders the tiny page shown in the OAuth popup; it notifies the
// opener window and closes itself.
func popupResult(success bool, detail string) []byte {
	status := "error"
	title := "Gagal menghubungkan"
	if success {
		status = "connected"
		title = "Akun Meta terhubung"
	}
	safe := htmlEscape(detail)
	html := `<!doctype html><html lang="id"><head><meta charset="utf-8"><title>` + title + `</title>
<style>body{font-family:system-ui,Segoe UI,sans-serif;background:#0f1722;color:#e8eef6;display:flex;
align-items:center;justify-content:center;height:100vh;margin:0}.box{text-align:center;max-width:360px;padding:24px}
.ico{font-size:42px}.t{font-size:18px;font-weight:600;margin:12px 0 6px}.d{opacity:.7;font-size:13px;word-break:break-word}</style></head>
<body><div class="box"><div class="ico">` + map[bool]string{true: "✅", false: "⚠️"}[success] + `</div>
<div class="t">` + title + `</div><div class="d">` + safe + `</div></div>
<script>try{window.opener&&window.opener.postMessage({source:"meta-oauth",status:"` + status + `",detail:` + jsonString(detail) + `},"*")}catch(e){}
setTimeout(function(){window.close()},1400);</script></body></html>`
	return []byte(html)
}
