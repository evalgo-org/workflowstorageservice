package main

import (
	"log"
	"os"

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

	log.Printf("workflowstorageservice starting on port %s", port)
	e.Logger.Fatal(e.Start(":" + port))
}
