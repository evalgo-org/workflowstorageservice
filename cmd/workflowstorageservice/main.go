package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"eve.evalgo.org/web"

	"eve.evalgo.org/common"
	evehttp "eve.evalgo.org/http"
	"eve.evalgo.org/registry"
	"eve.evalgo.org/semantic"
	"eve.evalgo.org/statemanager"
	"eve.evalgo.org/tracing"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	// Initialize logger
	logger := common.ServiceLogger("workflowstorageservice", "1.0.0")

	// Register action handlers with the semantic action registry
	// This allows the service to handle semantic actions without modifying switch statements
	semantic.MustRegister("UploadAction", handleSemanticStore)
	semantic.MustRegister("CreateAction", handleSemanticStore)
	semantic.MustRegister("StoreAction", handleSemanticStore)
	semantic.MustRegister("DownloadAction", handleSemanticRetrieve)
	semantic.MustRegister("RetrieveAction", handleSemanticRetrieve)
	semantic.MustRegister("FetchAction", handleSemanticRetrieve)

	e := echo.New()

	// Register EVE corporate identity assets
	web.RegisterAssets(e)
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Initialize tracing (gracefully disabled if unavailable)
	if tracer := tracing.Init(tracing.InitConfig{
		ServiceID:        "workflowstorageservice",
		DisableIfMissing: true,
	}); tracer != nil {
		e.Use(tracer.Middleware())
	}

	// EVE health check
	e.GET("/health", evehttp.HealthCheckHandler("workflowstorageservice", "1.0.0"))

	// Documentation endpoint
	e.GET("/v1/api/docs", evehttp.DocumentationHandler(evehttp.ServiceDocConfig{
		ServiceID:    "workflowstorageservice",
		ServiceName:  "Workflow Storage Service",
		Description:  "Storage and retrieval service for workflow definitions and data",
		Version:      "v1",
		Port:         8094,
		Capabilities: []string{"document-storage", "workflow-storage", "data-storage"},
		Endpoints: []evehttp.EndpointDoc{
			{
				Method:      "POST",
				Path:        "/v1/api/semantic/action",
				Description: "Execute storage operations via semantic actions (primary interface)",
			},
			{
				Method:      "POST",
				Path:        "/v1/api/workflows",
				Description: "Store workflow (REST convenience - converts to CreateAction)",
			},
			{
				Method:      "GET",
				Path:        "/v1/api/workflows/:id",
				Description: "Retrieve workflow (REST convenience - converts to RetrieveAction)",
			},
			{
				Method:      "PUT",
				Path:        "/v1/api/workflows/:id",
				Description: "Update workflow (REST convenience - converts to UpdateAction)",
			},
			{
				Method:      "DELETE",
				Path:        "/v1/api/workflows/:id",
				Description: "Delete workflow (REST convenience - converts to DeleteAction)",
			},
			{
				Method:      "POST",
				Path:        "/v1/api/store",
				Description: "Store workflow data (legacy)",
			},
			{
				Method:      "GET",
				Path:        "/v1/api/fetch/:key",
				Description: "Fetch workflow data by key (legacy)",
			},
			{
				Method:      "GET",
				Path:        "/health",
				Description: "Health check endpoint",
			},
		},
	}))

	// Initialize state manager
	sm := statemanager.New(statemanager.Config{
		ServiceName:   "workflowstorageservice",
		MaxOperations: 100,
	})

	// Register state endpoints
	apiGroup := e.Group("/v1/api")
	sm.RegisterRoutes(apiGroup)

	// Legacy API routes
	e.POST("/v1/api/store", handleStore)
	e.GET("/v1/api/fetch/:key", handleFetch)

	// EVE API Key middleware
	apiKey := os.Getenv("WORKFLOW_STORAGE_API_KEY")
	apiKeyMiddleware := evehttp.APIKeyMiddleware(apiKey)

	// Semantic action endpoint (primary interface)
	apiGroup.POST("/semantic/action", handleSemanticAction, apiKeyMiddleware)

	// REST endpoints (convenience adapters that convert to semantic actions)
	registerRESTEndpoints(apiGroup, apiKeyMiddleware)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8094"
	}

	// Get service URL from environment (for Docker container names) or default to localhost
	portInt, _ := strconv.Atoi(port)
	serviceURL := os.Getenv("WORKFLOWSTORAGE_SERVICE_URL")
	if serviceURL == "" {
		serviceURL = fmt.Sprintf("http://localhost:%d", portInt)
	}

	// Auto-register with registry service if REGISTRYSERVICE_API_URL is set
	_, err := registry.AutoRegister(registry.AutoRegisterConfig{
		ServiceID:    "workflowstorageservice",
		ServiceName:  "Workflow Storage Service",
		Description:  "Storage and retrieval service for workflow definitions and data",
		Port:         portInt,
		ServiceURL:   serviceURL,
		Directory:    "/home/opunix/workflowstorageservice",
		Binary:       "workflowstorageservice",
		Version:      "v1",
		Capabilities: []string{"document-storage", "workflow-storage", "data-storage"},
		APIVersions: []registry.APIVersion{
			{
				Version:       "v1",
				URL:           fmt.Sprintf("%s/v1", serviceURL),
				Documentation: fmt.Sprintf("%s/v1/api/docs", serviceURL),
				IsDefault:     true,
				Status:        "stable",
				ReleaseDate:   "2024-01-01",
				Capabilities:  []string{"document-storage", "workflow-storage", "data-storage"},
			},
		},
	})
	if err != nil {
		logger.WithError(err).Error("Failed to register with registry")
	}

	// Start server in goroutine
	go func() {
		logger.Infof("workflowstorageservice starting on port %s", port)
		if err := e.Start(":" + port); err != nil {
			logger.WithError(err).Error("Server error")
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Unregister from registry
	if err := registry.AutoUnregister("workflowstorageservice"); err != nil {
		logger.WithError(err).Error("Failed to unregister")
	}

	// Shutdown server
	if err := e.Close(); err != nil {
		logger.WithError(err).Error("Error during shutdown")
	}

	logger.Info("Server stopped")
}
