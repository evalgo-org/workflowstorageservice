package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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

	// Store the data directly (avoiding context creation issues)
	bucket := os.Getenv("HETZNER_S3_BUCKET")
	if bucket == "" {
		bucket = "px-semantic"
	}

	key := fmt.Sprintf("workflow-results/%s/%s.json", workflowID, action.Identifier)

	// Upload to S3
	dataBytes := []byte(data)
	_, err := s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(dataBytes),
		ContentType: aws.String(format),
	})
	if err != nil {
		log.Printf("Failed to upload to S3: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to store data"})
	}

	log.Printf("Stored workflow result via semantic action: %s (size: %d bytes)", key, len(dataBytes))

	// Return semantic response with Schema.org compliant structure
	response := StoreResponse{
		Type:           "DataDownload",
		ID:             fmt.Sprintf("#%s-result", action.Identifier),
		ContentURL:     fmt.Sprintf("s3://%s/%s", bucket, key),
		EncodingFormat: format,
		ContentSize:    int64(len(dataBytes)),
	}

	return c.JSON(http.StatusOK, response)
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

	// Fetch data from S3 directly
	bucket := os.Getenv("HETZNER_S3_BUCKET")
	if bucket == "" {
		bucket = "px-semantic"
	}

	// Download from S3
	result, err := s3Client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		log.Printf("Failed to fetch from S3: %v", err)
		return c.JSON(http.StatusNotFound, map[string]string{"error": "data not found"})
	}
	defer func() {
		if err := result.Body.Close(); err != nil {
			log.Printf("Failed to close S3 response body: %v", err)
		}
	}()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read data"})
	}

	contentType := "application/json"
	if result.ContentType != nil {
		contentType = *result.ContentType
	}

	log.Printf("Fetched workflow result via semantic action: %s (size: %d bytes)", key, len(data))

	// Check if result should be written to file
	var outputFile string

	// First check additionalProperty.outputFile
	if props, ok := rawAction["additionalProperty"].(map[string]interface{}); ok {
		if of, ok := props["outputFile"].(string); ok {
			outputFile = of
			log.Printf("DEBUG: Found outputFile in additionalProperty: %s", outputFile)
		}
	}

	// Fallback to result.contentUrl if present
	if outputFile == "" {
		if resultMap, ok := rawAction["result"].(map[string]interface{}); ok {
			if contentUrl, ok := resultMap["contentUrl"].(string); ok {
				outputFile = contentUrl
				log.Printf("DEBUG: Found contentUrl in result: %s", outputFile)
			} else {
				log.Printf("DEBUG: result exists but contentUrl not found or not a string: %#v", resultMap)
			}
		} else {
			log.Printf("DEBUG: result field not found in rawAction")
		}
	}

	// Check for outputType in additionalProperty
	outputType := "inline"
	if props, ok := rawAction["additionalProperty"].(map[string]interface{}); ok {
		if ot, ok := props["outputType"].(string); ok {
			outputType = ot
		}
	}

	// Write to file if outputFile is specified or outputType is "file"
	if outputFile != "" || outputType == "file" {
		// If no outputFile specified but outputType is "file", generate a default path
		if outputFile == "" {
			outputFile = fmt.Sprintf("/tmp/%s-result.dat", action.Identifier)
		}

		// Ensure parent directory exists
		dir := filepath.Dir(outputFile)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("Failed to create output directory: %v", err),
			})
		}

		// Write result to file (preserve semantic structure)
		if err := os.WriteFile(outputFile, data, 0644); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("Failed to write result to file: %v", err),
			})
		}

		log.Printf("Wrote workflow result to file: %s", outputFile)

		// Return response with file reference
		response := map[string]interface{}{
			"@context":     "https://schema.org",
			"@type":        "RetrieveAction",
			"identifier":   action.Identifier,
			"actionStatus": "CompletedActionStatus",
			"result": map[string]interface{}{
				"@type":          "MediaObject",
				"contentUrl":     outputFile,
				"encodingFormat": contentType,
				"contentSize":    int64(len(data)),
			},
		}

		return c.JSON(http.StatusOK, response)
	}

	// Default: return inline result
	response := FetchResponse{
		Data:           string(data),
		EncodingFormat: contentType,
		ContentSize:    int64(len(data)),
	}

	return c.JSON(http.StatusOK, response)
}
