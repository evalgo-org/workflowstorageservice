package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"eve.evalgo.org/common"
	evehttp "eve.evalgo.org/http"
	"eve.evalgo.org/pkg/statemanager"
	"eve.evalgo.org/registry"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	// Initialize logger
	logger := common.ServiceLogger("workflowstorageservice", "1.0.0")

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

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
				Description: "Execute storage operations via semantic actions",
			},
			{
				Method:      "POST",
				Path:        "/v1/api/store",
				Description: "Store workflow data",
			},
			{
				Method:      "GET",
				Path:        "/v1/api/fetch/:key",
				Description: "Fetch workflow data by key",
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

	// API routes
	e.POST("/v1/api/store", handleStore)
	e.GET("/v1/api/fetch/:key", handleFetch)

	// Semantic API endpoint with EVE API key middleware
	apiKey := os.Getenv("WORKFLOW_STORAGE_API_KEY")
	apiKeyMiddleware := evehttp.APIKeyMiddleware(apiKey)
	e.POST("/v1/api/semantic/action", handleSemanticAction, apiKeyMiddleware)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8094"
	}

	// Auto-register with registry service if REGISTRYSERVICE_API_URL is set
	portInt, _ := strconv.Atoi(port)
	_, err := registry.AutoRegister(registry.AutoRegisterConfig{
		ServiceID:    "workflowstorageservice",
		ServiceName:  "Workflow Storage Service",
		Description:  "Storage and retrieval service for workflow definitions and data",
		Port:         portInt,
		Directory:    "/home/opunix/workflowstorageservice",
		Binary:       "workflowstorageservice",
		Version:      "v1",
		Capabilities: []string{"document-storage", "workflow-storage", "data-storage"},
		APIVersions: []registry.APIVersion{
			{
				Version:       "v1",
				URL:           fmt.Sprintf("http://localhost:%d/v1", portInt),
				Documentation: fmt.Sprintf("http://localhost:%d/v1/api/docs", portInt),
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
