package handler

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"marketingflow/internal/authmw"
	"marketingflow/internal/config"
	"marketingflow/internal/gsheets"
	"marketingflow/internal/middleware"
	"marketingflow/internal/model"
	"marketingflow/internal/repository"
	"marketingflow/internal/service"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NewRouter wires repositories, services, handlers and routes together.
func NewRouter(db *gorm.DB, cfg *config.Config) *gin.Engine {
	// Repositories
	userRepo := repository.NewUserRepository(db)
	itemRepo := repository.NewWorkItemRepository(db)
	stepRepo := repository.NewStepRepository(db)
	docRepo := repository.NewDocumentRepository(db)
	metaRepo := repository.NewMetaRepository(db)

	// Infrastructure
	tokenMgr := middleware.NewTokenManager(cfg.JWTSecret, cfg.JWTExpiryHours)
	// Accept the unified dashboard's Ed25519 SSO login token (one login, no
	// bridge) when AUTH_JWKS_URL is set; native marketing JWT still works.
	ssoV := authmw.New(authmw.Options{JWKSURL: os.Getenv("AUTH_JWKS_URL"), Issuer: os.Getenv("AUTH_ISSUER")})

	// Services
	authSvc := service.NewAuthService(userRepo, tokenMgr)
	itemSvc := service.NewWorkItemService(itemRepo, stepRepo)
	stepSvc := service.NewStepService(stepRepo, itemSvc)
	docSvc := service.NewDocumentService(docRepo, cfg.UploadDir)
	dashboardSvc := service.NewDashboardService(stepRepo)
	contentPlanSvc := service.NewContentPlanService(itemRepo)

	// Google Sheets client for the Content Plan sync (nil → public XLSX export).
	sheetsClient, err := gsheets.New(cfg.GoogleCredentials)
	if err != nil {
		log.Printf("content-plan: kredensial Google diabaikan: %v", err)
	}

	// Handlers
	authH := NewAuthHandler(authSvc)
	itemH := NewWorkItemHandler(itemSvc, docSvc)
	stepH := NewStepHandler(stepSvc, docSvc)
	dashboardH := NewDashboardHandler(dashboardSvc)
	metaH := NewMetaHandler(metaRepo, cfg.MetaToken, cfg.MetaAPIVersion, cfg.MetaBusinessID, cfg.MetaAdAccount)
	metaOAuthH := NewMetaOAuthHandler(metaRepo, tokenMgr, cfg)

	hub := NewRealtimeHub()
	contentPlanH := NewContentPlanHandler(contentPlanSvc, sheetsClient, cfg.ContentSheetID, hub)
	contentPlanH.StartAutoSync(context.Background())

	r := gin.Default()
	r.MaxMultipartMemory = 32 << 20 // 32 MiB

	r.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORSOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	api := r.Group("/api")
	{
		api.POST("/auth/login", authH.Login)
		// Realtime push: validates its own ?token= (browsers can't set WS headers).
		api.GET("/ws", hub.ServeWS(tokenMgr, ssoV))

		// Meta OAuth (Facebook Login) entry + callback are top-level navigations
		// (popup / Facebook redirect) so they can't carry a bearer header:
		// Login validates its own ?token=, Callback is validated by signed state.
		api.GET("/meta/oauth/login", metaOAuthH.Login)
		api.GET("/meta/oauth/callback", metaOAuthH.Callback)

		authed := api.Group("")
		authed.Use(middleware.Auth(tokenMgr, ssoV))
		// Bump the realtime revision on every successful write so all connected
		// dashboards refresh instantly.
		authed.Use(hub.BumpMiddleware())
		{
			authed.GET("/auth/me", authH.Me)

			// Reference metadata (alur labels) for the UI.
			authed.GET("/meta/alur", func(c *gin.Context) {
				c.JSON(http.StatusOK, service.AlurLabels)
			})

			// Work items & steps (Alur A–D).
			authed.GET("/work-items", itemH.List)
			authed.POST("/work-items", itemH.Create)
			// Destructive: wipe all work data (keeps accounts). Kadep only.
			authed.POST("/work-items/reset", middleware.RequireRole(model.RoleKadep), itemH.Reset)
			authed.GET("/work-items/:id", itemH.Get)
			authed.GET("/work-items/:id/progress", itemH.Progress)

			// Content Plan sync (Google Sheets → work items) + background auto-sync.
			authed.GET("/content-plan/source", contentPlanH.Source)
			authed.POST("/content-plan/sync/preview", contentPlanH.Preview)
			authed.POST("/content-plan/sync/approve", contentPlanH.Approve)
			authed.GET("/content-plan/auto", contentPlanH.AutoStatus)
			authed.POST("/content-plan/auto", contentPlanH.AutoSet)

			authed.GET("/my-steps", stepH.Mine)
			authed.GET("/steps/:id", stepH.Get)
			authed.PUT("/steps/:id", stepH.Update)
			authed.POST("/steps/:id/documents", stepH.UploadDocument)
			authed.GET("/documents/:id/download", stepH.DownloadDocument)

			// Dashboard: early warning feed.
			authed.GET("/dashboard/warnings", dashboardH.EarlyWarnings)

			// Meta (Facebook) live data — Ads / WhatsApp / Instagram tabs.
			authed.GET("/meta/ads", metaH.Ads)
			authed.GET("/meta/ads/detail", metaH.AdsDetail)
			authed.GET("/meta/ads/campaign", metaH.AdsCampaign)
			authed.GET("/meta/whatsapp", metaH.WhatsApp)
			authed.GET("/meta/instagram", metaH.Instagram)
			authed.GET("/meta/instagram/conversations", metaH.IGConversations)
			authed.GET("/meta/instagram/messages", metaH.IGMessages)
			authed.POST("/meta/instagram/send", metaH.IGSend)

			// Meta OAuth app config + connected-account management (multi-account).
			authed.GET("/meta/oauth/config", metaOAuthH.Config)
			authed.PUT("/meta/oauth/config", metaOAuthH.SaveConfig)
			authed.POST("/meta/connections/manual", metaOAuthH.ConnectManual)
			authed.GET("/meta/connections", metaOAuthH.ListConnections)
			authed.PATCH("/meta/connections/:id", metaOAuthH.UpdateConnection)
			authed.POST("/meta/connections/:id/activate", metaOAuthH.Activate)
			authed.DELETE("/meta/connections/:id", metaOAuthH.Disconnect)
		}
	}

	return r
}
