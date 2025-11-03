package main

import (
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"eve.evalgo.org/registry"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Health check
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{
			"service": "workflowstorageservice",
			"status":  "healthy",
		})
	})

	// API routes
	e.POST("/v1/api/store", handleStore)
	e.GET("/v1/api/fetch/:key", handleFetch)
	e.POST("/v1/api/semantic/action", handleSemanticAction)

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
		Capabilities: []string{"workflow-storage", "data-storage", "semantic-actions"},
	})
	if err != nil {
		log.Printf("Failed to register with registry: %v", err)
	}

	// Start server in goroutine
	go func() {
		log.Printf("workflowstorageservice starting on port %s", port)
		if err := e.Start(":" + port); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Unregister from registry
	if err := registry.AutoUnregister("workflowstorageservice"); err != nil {
		log.Printf("Failed to unregister: %v", err)
	}

	// Shutdown server
	if err := e.Close(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	log.Println("Server stopped")
}
