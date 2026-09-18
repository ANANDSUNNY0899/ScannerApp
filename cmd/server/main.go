package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/scannerapp/backend/internal/config"
	"github.com/scannerapp/backend/internal/handler"
	"github.com/scannerapp/backend/internal/middleware"
	"github.com/scannerapp/backend/internal/repository/postgres"
	"github.com/scannerapp/backend/internal/repository/redis"
	"github.com/scannerapp/backend/internal/service"
	"github.com/scannerapp/backend/internal/storage"
	"github.com/scannerapp/backend/pkg/jwt"
	"github.com/scannerapp/backend/pkg/response"
)

func main() {
	cfg := config.LoadConfig()
	log.Printf("Starting Document Scanner API on port %s...", cfg.Port)

	// Database Connection
	db, err := postgres.NewPostgresDB(cfg.PostgresURL)
	if err != nil {
		log.Printf("Warning: PostgreSQL not connected (%v). Using degraded mode if offline.", err)
	} else {
		defer db.Close()
		log.Println("Successfully connected to PostgreSQL.")

		if err := postgres.RunMigrations(db, "migrations"); err != nil {
			log.Printf("Warning: Database migrations failed: %v", err)
		} else {
			log.Println("Database migrations applied successfully.")
		}
	}

	// Redis Connection
	rdb, err := redis.NewRedisClient(cfg.RedisAddr, cfg.RedisPass)
	if err != nil {
		log.Printf("Warning: Redis not connected (%v). Rate limiting requires Redis.", err)
	} else {
		defer rdb.Close()
		log.Println("Successfully connected to Redis.")
	}

	// Token Manager
	tokenManager := jwt.NewTokenManager(cfg.JWTSecret, cfg.JWTHours)

	// Storage Service (MinIO / S3)
	s3Storage, err := storage.NewS3Storage(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3UseSSL)
	if err != nil {
		log.Printf("Warning: Failed to initialize MinIO storage (%v).", err)
	} else {
		log.Printf("Connected to MinIO object storage at %s.", cfg.S3Endpoint)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s3Storage.EnsureBucket(ctx, cfg.S3Bucket); err != nil {
			log.Printf("Warning: EnsureBucket %s error: %v", cfg.S3Bucket, err)
		} else {
			log.Printf("Bucket %s is ready.", cfg.S3Bucket)
		}
		cancel()
	}

	// Repositories
	userRepo := postgres.NewUserRepository(db)
	syncRepo := postgres.NewSyncRepository(db)
	receiptRepo := postgres.NewReceiptRepository(db)
	limiterRepo := redis.NewLimiterRepository(rdb)

	// Services
	authService := service.NewAuthService(userRepo, tokenManager)
	syncService := service.NewSyncService(syncRepo)
	ocrService := service.NewOCRService(limiterRepo, cfg.MaxFreeOCR)
	billingService, _ := service.NewPlayBillingService(context.Background())
	geminiService := service.NewGeminiService(cfg.GeminiAPIKey, cfg.GeminiModel)
	receiptService := service.NewReceiptService(receiptRepo, geminiService, s3Storage, cfg.S3Bucket)

	// Handlers
	authHandler := handler.NewAuthHandler(authService)
	syncHandler := handler.NewSyncHandler(syncService, s3Storage, cfg.S3Bucket)
	ocrHandler := handler.NewOCRHandler(ocrService, userRepo)
	subscriptionHandler := handler.NewSubscriptionHandler(billingService, userRepo)
	receiptHandler := handler.NewReceiptHandler(receiptService)

	// Router Setup
	r := mux.NewRouter()
	r.Use(middleware.LoggerMiddleware)

	// Health Checks
	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, http.StatusOK, map[string]string{
			"status":  "healthy",
			"service": "scanner-backend",
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	}
	r.HandleFunc("/", healthHandler).Methods(http.MethodGet)
	r.HandleFunc("/health", healthHandler).Methods(http.MethodGet)

	api := r.PathPrefix("/api/v1").Subrouter()

	// Public Auth Routes
	authRouter := api.PathPrefix("/auth").Subrouter()
	authRouter.HandleFunc("/register", authHandler.Register).Methods(http.MethodPost)
	authRouter.HandleFunc("/login", authHandler.Login).Methods(http.MethodPost)

	// Protected Routes
	protected := api.PathPrefix("").Subrouter()
	protected.Use(middleware.AuthMiddleware(tokenManager))

	// Auth Profile
	protected.HandleFunc("/auth/me", authHandler.GetMe).Methods(http.MethodGet)

	// Sync Routes
	protected.HandleFunc("/sync/push", syncHandler.Push).Methods(http.MethodPost)
	protected.HandleFunc("/sync/pull", syncHandler.Pull).Methods(http.MethodGet)
	protected.HandleFunc("/documents/{id}/file", syncHandler.UploadFile).Methods(http.MethodPost)
	protected.HandleFunc("/documents/{id}", syncHandler.GetDocument).Methods(http.MethodGet)

	// OCR Routes (Enforces 5 max for free users)
	protected.HandleFunc("/ocr/process", ocrHandler.Process).Methods(http.MethodPost)
	protected.HandleFunc("/ocr/quota", ocrHandler.GetQuota).Methods(http.MethodGet)

	// Subscription Routes
	protected.HandleFunc("/subscriptions/verify", subscriptionHandler.Verify).Methods(http.MethodPost)

	// Universal Multi-Tenant Receipt Extraction & Ledger Routes
	protected.HandleFunc("/receipts/extract", receiptHandler.Extract).Methods(http.MethodPost)
	protected.HandleFunc("/receipts", receiptHandler.List).Methods(http.MethodGet)
	protected.HandleFunc("/receipts/summary", receiptHandler.Summary).Methods(http.MethodGet)
	protected.HandleFunc("/receipts/export/csv", receiptHandler.ExportCSV).Methods(http.MethodGet)
	protected.HandleFunc("/receipts/{id}", receiptHandler.GetByID).Methods(http.MethodGet)
	protected.HandleFunc("/receipts/{id}", receiptHandler.Update).Methods(http.MethodPut)
	protected.HandleFunc("/receipts/{id}", receiptHandler.Delete).Methods(http.MethodDelete)

	// Server port configuration (Render dynamically sets $PORT)
	port := os.Getenv("PORT")
	if port == "" {
		port = cfg.Port
	}
	if port == "" {
		port = "8080"
	}

	// Server config
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Server listening on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server listen error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped cleanly.")
}
