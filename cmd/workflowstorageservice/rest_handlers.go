package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
)

// REST endpoint request types

type StoreWorkflowRequest struct {
	ID         string                 `json:"id"`
	Definition map[string]interface{} `json:"definition"`
	Format     string                 `json:"format,omitempty"`
}

type UpdateWorkflowRequest struct {
	Definition map[string]interface{} `json:"definition"`
	Format     string                 `json:"format,omitempty"`
}

// registerRESTEndpoints adds REST endpoints that convert to semantic actions
func registerRESTEndpoints(apiGroup *echo.Group, apiKeyMiddleware echo.MiddlewareFunc) {
	// POST /v1/api/workflows - Store workflow
	apiGroup.POST("/workflows", storeWorkflowREST, apiKeyMiddleware)

	// GET /v1/api/workflows/:id - Retrieve workflow
	apiGroup.GET("/workflows/:id", getWorkflowREST, apiKeyMiddleware)

	// PUT /v1/api/workflows/:id - Update workflow
	apiGroup.PUT("/workflows/:id", updateWorkflowREST, apiKeyMiddleware)

	// DELETE /v1/api/workflows/:id - Delete workflow
	apiGroup.DELETE("/workflows/:id", deleteWorkflowREST, apiKeyMiddleware)
}

// storeWorkflowREST handles REST POST /v1/api/workflows
func storeWorkflowREST(c echo.Context) error {
	var req StoreWorkflowRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Invalid request: %v", err)})
	}

	if req.ID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id is required"})
	}
	if req.Definition == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "definition is required"})
	}

	// Convert definition to JSON string
	definitionJSON, err := json.Marshal(req.Definition)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to marshal definition: %v", err)})
	}

	// Determine format
	format := req.Format
	if format == "" {
		format = "application/json"
	}

	// Convert to JSON-LD CreateAction
	action := map[string]interface{}{
		"@context":   "https://schema.org",
		"@type":      "CreateAction",
		"identifier": req.ID,
		"object": map[string]interface{}{
			"@type":          "DigitalDocument",
			"text":           string(definitionJSON),
			"encodingFormat": format,
		},
	}

	return callSemanticHandler(c, action)
}

// getWorkflowREST handles REST GET /v1/api/workflows/:id
func getWorkflowREST(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id is required"})
	}

	// Get bucket from environment or use default
	bucket := c.QueryParam("bucket")
	if bucket == "" {
		bucket = "px-semantic"
	}

	// Construct S3 URL
	s3URL := fmt.Sprintf("s3://%s/workflow-results/default/%s.json", bucket, id)

	// Convert to JSON-LD RetrieveAction
	action := map[string]interface{}{
		"@context":   "https://schema.org",
		"@type":      "RetrieveAction",
		"identifier": id,
		"object": map[string]interface{}{
			"@type":      "DigitalDocument",
			"contentUrl": s3URL,
		},
	}

	return callSemanticHandler(c, action)
}

// updateWorkflowREST handles REST PUT /v1/api/workflows/:id
func updateWorkflowREST(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id is required"})
	}

	var req UpdateWorkflowRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Invalid request: %v", err)})
	}

	if req.Definition == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "definition is required"})
	}

	// Convert definition to JSON string
	definitionJSON, err := json.Marshal(req.Definition)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to marshal definition: %v", err)})
	}

	// Determine format
	format := req.Format
	if format == "" {
		format = "application/json"
	}

	// Convert to JSON-LD UpdateAction
	action := map[string]interface{}{
		"@context":   "https://schema.org",
		"@type":      "UpdateAction",
		"identifier": id,
		"object": map[string]interface{}{
			"@type":          "DigitalDocument",
			"text":           string(definitionJSON),
			"encodingFormat": format,
		},
	}

	return callSemanticHandler(c, action)
}

// deleteWorkflowREST handles REST DELETE /v1/api/workflows/:id
func deleteWorkflowREST(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id is required"})
	}

	// Get bucket from environment or use default
	bucket := c.QueryParam("bucket")
	if bucket == "" {
		bucket = "px-semantic"
	}

	// Construct S3 URL
	s3URL := fmt.Sprintf("s3://%s/workflow-results/default/%s.json", bucket, id)

	// Convert to JSON-LD DeleteAction
	action := map[string]interface{}{
		"@context":   "https://schema.org",
		"@type":      "DeleteAction",
		"identifier": id,
		"object": map[string]interface{}{
			"@type":      "DigitalDocument",
			"contentUrl": s3URL,
		},
	}

	return callSemanticHandler(c, action)
}

// callSemanticHandler converts action to JSON and calls the semantic action handler
func callSemanticHandler(c echo.Context, action map[string]interface{}) error {
	// Marshal action to JSON
	actionJSON, err := json.Marshal(action)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to marshal action: %v", err)})
	}

	// Create new request with JSON-LD body
	newReq := c.Request().Clone(c.Request().Context())
	newReq.Body = io.NopCloser(bytes.NewReader(actionJSON))
	newReq.Header.Set("Content-Type", "application/json")

	// Create new context with modified request
	newCtx := c.Echo().NewContext(newReq, c.Response())
	newCtx.SetPath(c.Path())
	newCtx.SetParamNames(c.ParamNames()...)
	newCtx.SetParamValues(c.ParamValues()...)

	// Call the existing semantic action handler
	return handleSemanticAction(newCtx)
}
