package handler

import (
	"net/http"
	"time"

	"marketingflow/internal/config"
	"marketingflow/internal/middleware"
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

	// Infrastructure
	tokenMgr := middleware.NewTokenManager(cfg.JWTSecret, cfg.JWTExpiryHours)

	// Services
	authSvc := service.NewAuthService(userRepo, tokenMgr)
	itemSvc := service.NewWorkItemService(itemRepo, stepRepo)
	stepSvc := service.NewStepService(stepRepo, itemSvc)
	docSvc := service.NewDocumentService(docRepo, cfg.UploadDir)
	dashboardSvc := service.NewDashboardService(stepRepo)

	// Handlers
	authH := NewAuthHandler(authSvc)
	itemH := NewWorkItemHandler(itemSvc)
	stepH := NewStepHandler(stepSvc, docSvc)
	dashboardH := NewDashboardHandler(dashboardSvc)
	metaH := NewMetaHandler(cfg.MetaToken, cfg.MetaAPIVersion, cfg.MetaBusinessID, cfg.MetaAdAccount)

	r := gin.Default()
	r.MaxMultipartMemory = 32 << 20 // 32 MiB

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://localhost:5174", "http://localhost:5177", "http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	hub := NewRealtimeHub()

	api := r.Group("/api")
	{
		api.POST("/auth/login", authH.Login)
		// Realtime push: validates its own ?token= (browsers can't set WS headers).
		api.GET("/ws", hub.ServeWS(tokenMgr))

		authed := api.Group("")
		authed.Use(middleware.Auth(tokenMgr))
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
			authed.GET("/work-items/:id", itemH.Get)
			authed.GET("/work-items/:id/progress", itemH.Progress)

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
			authed.GET("/meta/whatsapp", metaH.WhatsApp)
			authed.GET("/meta/instagram", metaH.Instagram)
		}
	}

	return r
}
