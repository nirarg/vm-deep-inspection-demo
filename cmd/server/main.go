package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nirarg/v2v-vm-validations/pkg/persistent"
	"github.com/nirarg/vm-deep-inspection-demo/internal/api"
	"github.com/nirarg/vm-deep-inspection-demo/internal/config"
	"github.com/nirarg/vm-deep-inspection-demo/internal/storage"
	"github.com/nirarg/vm-deep-inspection-demo/internal/vmware"
	"github.com/sirupsen/logrus"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	_ "github.com/nirarg/vm-deep-inspection-demo/docs"
)

// @title VM Deep Inspection Demo API
// @version 0.1
// @description A Go service for investigating "Deep inspection" of VMs in VMware vSphere 
// @host localhost:8080
// @BasePath /
// @schemes http https

func main() {
	// Parse command line flags
	var configFile string
	flag.StringVar(&configFile, "config", "", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Setup logger based on configuration
	log := setupLogger(cfg.Logging)
	log.Info("Starting VM Deep Inspection Demo service...")
	log.WithField("config_file", configFile).Debug("Configuration loaded")

	// Set Gin mode based on log level
	if cfg.Logging.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize VMware client
	vmwareClient := vmware.NewClient(cfg.VMware, log)

	// Connect to vCenter
	ctx := context.Background()
	if err := vmwareClient.Connect(ctx); err != nil {
		log.WithError(err).Warn("Failed to connect to vCenter at startup, will retry on first request")
	} else {
		log.Info("Successfully connected to vCenter")
	}

	// Initialize VMware services
	vmService := vmware.NewVMService(vmwareClient, log)

	// Initialize database connection
	db, err := initDatabase(cfg.Database, log)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.WithFields(logrus.Fields{
		"type": cfg.Database.Type,
		"name": cfg.Database.Name,
	}).Info("Database initialized")

	// Initialize inspection database
	inspectionDB, err := storage.NewInspectionDB(db, log)
	if err != nil {
		log.Fatalf("Failed to initialize inspection database: %v", err)
	}
	log.Info("Inspection database schema migrated")

	// Initialize persistent inspector with credentials and DB
	credentials := persistent.Credentials{
		VCenterURL: cfg.VMware.VCenterURL,
		Username:   cfg.VMware.Username,
		Password:   cfg.VMware.Password,
	}
	inspector := persistent.NewInspector(
		"",    // virt-inspector path (uses system PATH)
		"",    // virt-v2v-inspector path (uses system PATH)
		30*time.Minute, // timeout
		credentials,
		log,
		inspectionDB, // Use file-based DB persistence
	)

	// Initialize handlers
	vmHandler := api.NewVMHandler(vmService, vmwareClient, inspector, log)

	// Setup router
	router := gin.Default()

	// CORS middleware (if enabled)
	if cfg.Server.EnableCORS {
		router.Use(corsMiddleware())
	}

	// Request logging middleware
	router.Use(requestLoggerMiddleware(log))

	// Health check endpoint
	router.GET("/health", healthCheck(log))

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// VM routes
		v1.GET("/vms", vmHandler.ListVMs)
		v1.GET("/vms/:name", vmHandler.GetVM)
		v1.POST("/vms/snapshot", vmHandler.CreateVMSnapshot)

		// Clone and inspection routes
		v1.POST("/vms/clone", vmHandler.CreateClone)
		v1.DELETE("/vms/delete-clone", vmHandler.DeleteClone)

		// Snapshot inspection route (direct inspection without clone)
		v1.POST("/vms/inspect-snapshot", vmHandler.InspectSnapshot)

		// Validation checks route (generic check runner)
		v1.POST("/vms/check", vmHandler.RunCheck)
	}

	// Swagger documentation endpoint
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Create HTTP server with configuration
	server := &http.Server{
		Addr:         cfg.Server.GetAddress(),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start server in a goroutine
	go func() {
		log.WithFields(logrus.Fields{
			"address": cfg.Server.GetAddress(),
			"tls":     cfg.Server.IsTLSEnabled(),
		}).Info("Server starting")

		log.Infof("Swagger UI available at: http%s://%s/swagger/index.html",
			map[bool]string{true: "s", false: ""}[cfg.Server.IsTLSEnabled()],
			cfg.Server.GetAddress())

		var err error
		if cfg.Server.IsTLSEnabled() {
			err = server.ListenAndServeTLS(cfg.Server.TLSConfig.CertFile, cfg.Server.TLSConfig.KeyFile)
		} else {
			err = server.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("Shutting down server...")

	// Give the server time to finish handling existing requests
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	// Close database connection
	sqlDB, err := db.DB()
	if err == nil {
		if err := sqlDB.Close(); err != nil {
			log.WithError(err).Warn("Error closing database connection")
		} else {
			log.Info("Database connection closed")
		}
	}

	// Disconnect from vCenter
	disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer disconnectCancel()
	if err := vmwareClient.Disconnect(disconnectCtx); err != nil {
		log.WithError(err).Warn("Error disconnecting from vCenter")
	}

	log.Info("Server exited")
}

func setupLogger(cfg config.LoggingConfig) *logrus.Logger {
	log := logrus.New()

	// Set log level
	level, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	log.SetLevel(level)

	// Set log format
	if cfg.Format == "json" {
		log.SetFormatter(&logrus.JSONFormatter{})
	} else {
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
		})
	}

	// Set output
	switch cfg.Output {
	case "stderr":
		log.SetOutput(os.Stderr)
	case "file":
		if cfg.FilePath != "" {
			file, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to open log file %s: %v\n", cfg.FilePath, err)
				log.SetOutput(os.Stdout)
			} else {
				log.SetOutput(file)
			}
		}
	default:
		log.SetOutput(os.Stdout)
	}

	return log
}

// corsMiddleware returns a CORS middleware
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// requestLoggerMiddleware logs HTTP requests
func requestLoggerMiddleware(log *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Process request
		c.Next()

		// Log request
		latency := time.Since(start)
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()

		if raw != "" {
			path = path + "?" + raw
		}

		entry := log.WithFields(logrus.Fields{
			"status":     statusCode,
			"latency":    latency,
			"client_ip":  clientIP,
			"method":     method,
			"path":       path,
			"user_agent": c.Request.UserAgent(),
		})

		if len(c.Errors) > 0 {
			entry.Error(c.Errors.String())
		} else {
			entry.Info("Request processed")
		}
	}
}

// healthCheck returns a simple health check handler
func healthCheck(log *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now(),
			"service":   "vm-deep-inspection-demo",
			"version":   "1.0.0",
		})
	}
}

// initDatabase initializes and returns a GORM database connection
func initDatabase(cfg config.DatabaseConfig, log *logrus.Logger) (*gorm.DB, error) {
	var dialector gorm.Dialector

	dsn := cfg.GetDSN()
	if dsn == "" {
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}

	// Select the appropriate GORM driver based on database type
	switch cfg.Type {
	case "sqlite":
		dialector = sqlite.Open(dsn)
	case "postgres":
		dialector = postgres.Open(dsn)
	case "mysql":
		dialector = mysql.Open(dsn)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}

	// Configure GORM logger to only log errors (suppress "record not found" messages)
	gormLogger := logger.Default.LogMode(logger.Error)

	// Open database connection
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: gormLogger, // Only log errors, not info messages
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying sql.DB to configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying database: %w", err)
	}

	// Set connection pool settings
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}
