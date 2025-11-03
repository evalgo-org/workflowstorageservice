package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// SemanticStoreAction represents a Store/Upload action
type SemanticStoreAction struct {
	Context    string                 `json:"@context,omitempty"`
	Type       string                 `json:"@type"`
	Identifier string                 `json:"identifier"`
	Name       string                 `json:"name,omitempty"`
	Object     *SemanticMediaObject   `json:"object,omitempty"`
	Target     map[string]interface{} `json:"target,omitempty"`
}

// SemanticMediaObject represents data to be stored
type SemanticMediaObject struct {
	Type           string `json:"@type,omitempty"`
	ContentURL     string `json:"contentUrl,omitempty"`
	EncodingFormat string `json:"encodingFormat,omitempty"`
	Text           string `json:"text,omitempty"` // Inline data
}

// SemanticRetrieveAction represents a Retrieve/Download action (Schema.org compliant)
type SemanticRetrieveAction struct {
	Context    string               `json:"@context,omitempty"`
	Type       string               `json:"@type"`
	Identifier string               `json:"identifier"`
	Name       string               `json:"name,omitempty"`
	Object     *SemanticMediaObject `json:"object,omitempty"` // What to retrieve (resource s3:// location)
	Target     interface{}          `json:"target,omitempty"` // Where to execute (service endpoint) - optional, for future use
	Result     *SemanticMediaObject `json:"result,omitempty"`
}

func handleSemanticAction(c echo.Context) error {
	// Parse raw JSON to detect action type
	var rawAction map[string]interface{}
	if err := c.Bind(&rawAction); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON-LD"})
	}

	actionType, ok := rawAction["@type"].(string)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "@type is required"})
	}

	switch actionType {
	case "UploadAction", "CreateAction", "StoreAction":
		return handleSemanticStore(c, rawAction)
	case "DownloadAction", "RetrieveAction", "FetchAction":
		return handleSemanticRetrieve(c, rawAction)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("unsupported action type: %s", actionType),
		})
	}
}

func handleSemanticStore(c echo.Context, rawAction map[string]interface{}) error {
	actionBytes, _ := json.Marshal(rawAction)
	var action SemanticStoreAction
	if err := json.Unmarshal(actionBytes, &action); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid action structure"})
	}

	// Extract workflow context from properties or headers
	workflowID := c.Request().Header.Get("X-Workflow-ID")
	if workflowID == "" {
		workflowID = "default"
	}

	// Get data to store
	var data string
	var format string

	if action.Object != nil {
		if action.Object.Text != "" {
			data = action.Object.Text
		} else if action.Object.ContentURL != "" {
			// TODO: Fetch from URL
			return c.JSON(http.StatusNotImplemented, map[string]string{
				"error": "fetching from contentUrl not yet implemented",
			})
		}
		format = action.Object.EncodingFormat
	}

	if data == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no data to store"})
	}

	if format == "" {
		format = "application/json"
	}

	// Store the data
	storeReq := StoreRequest{
		WorkflowID: workflowID,
		ActionID:   action.Identifier,
		Data:       data,
		Format:     format,
	}

	reqBytes, _ := json.Marshal(storeReq)

	// Create new request context for store handler
	req, _ := http.NewRequest("POST", "/v1/api/store", bytes.NewReader(reqBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := c.Response().Writer

	e := c.Echo()
	ctx := e.NewContext(req, rec.(*echo.Response))

	return handleStore(ctx)
}

func handleSemanticRetrieve(c echo.Context, rawAction map[string]interface{}) error {
	actionBytes, _ := json.Marshal(rawAction)
	var action SemanticRetrieveAction
	if err := json.Unmarshal(actionBytes, &action); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid action structure"})
	}

	// Extract s3:// URL from object (correct Schema.org)
	var contentURL string

	if action.Object != nil && action.Object.ContentURL != "" {
		contentURL = action.Object.ContentURL
	}

	if contentURL == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "object.contentUrl is required (resource s3:// location)",
		})
	}

	// Parse s3:// URL
	// Format: s3://bucket/workflow-results/workflowId/actionId.json
	if len(contentURL) < 6 || contentURL[:5] != "s3://" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "only s3:// URLs supported"})
	}

	// Remove s3://bucket/ prefix to get key
	parts := strings.Split(contentURL[5:], "/")
	if len(parts) < 2 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid s3 URL format"})
	}

	key := strings.Join(parts[1:], "/")

	// Create new context with key parameter
	req, _ := http.NewRequest("GET", "/v1/api/fetch/"+key, nil)
	rec := c.Response().Writer

	e := c.Echo()
	ctx := e.NewContext(req, rec.(*echo.Response))
	ctx.SetParamNames("key")
	ctx.SetParamValues(key)

	return handleFetch(ctx)
}
