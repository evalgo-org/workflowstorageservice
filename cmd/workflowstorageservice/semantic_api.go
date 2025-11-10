package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"eve.evalgo.org/semantic"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/labstack/echo/v4"
)

func handleSemanticAction(c echo.Context) error {
	// Parse semantic action
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(c.Request().Body); err != nil {
		return semantic.ReturnActionError(c, nil, "Failed to read request body", err)
	}
	bodyBytes := buf.Bytes()

	action, err := semantic.ParseSemanticAction(bodyBytes)
	if err != nil {
		return semantic.ReturnActionError(c, nil, "Failed to parse semantic action", err)
	}

	// Dispatch to registered handler using the ActionRegistry
	// No switch statement needed - handlers are registered at startup
	return semantic.Handle(c, action)
}

func handleSemanticStoreImpl(c echo.Context, action *semantic.SemanticAction) error {
	// Extract workflow context from properties or headers
	workflowID := c.Request().Header.Get("X-Workflow-ID")
	if workflowID == "" {
		workflowID = "default"
	}

	// Get data to store
	if action.Object == nil {
		return semantic.ReturnActionError(c, action, "object is required", nil)
	}

	var data string
	var format string

	if action.Object.Text != "" {
		data = action.Object.Text
	} else if action.Object.ContentUrl != "" {
		// TODO: Fetch from URL
		return semantic.ReturnActionError(c, action, "fetching from contentUrl not yet implemented", nil)
	}

	format = action.Object.EncodingFormat
	if format == "" {
		format = "application/json"
	}

	if data == "" {
		return semantic.ReturnActionError(c, action, "no data to store", nil)
	}

	// Store the data
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
		return semantic.ReturnActionError(c, action, "Failed to store data", err)
	}

	log.Printf("Stored workflow result via semantic action: %s (size: %d bytes)", key, len(dataBytes))

	// Use semantic Result structure
	action.Result = &semantic.SemanticResult{
		Type:   "DigitalDocument",
		Format: format,
		Value: map[string]interface{}{
			"contentUrl":     fmt.Sprintf("s3://%s/%s", bucket, key),
			"encodingFormat": format,
			"contentSize":    int64(len(dataBytes)),
		},
	}

	semantic.SetSuccessOnAction(action)
	return c.JSON(http.StatusOK, action)
}

func handleSemanticRetrieveImpl(c echo.Context, action *semantic.SemanticAction) error {
	// Extract s3:// URL from object
	if action.Object == nil {
		return semantic.ReturnActionError(c, action, "object is required", nil)
	}

	contentURL := action.Object.ContentUrl
	if contentURL == "" {
		return semantic.ReturnActionError(c, action, "object.contentUrl is required (resource s3:// location)", nil)
	}

	// Parse s3:// URL
	// Format: s3://bucket/workflow-results/workflowId/actionId.json
	if len(contentURL) < 6 || contentURL[:5] != "s3://" {
		return semantic.ReturnActionError(c, action, "only s3:// URLs supported", nil)
	}

	// Remove s3://bucket/ prefix to get key
	parts := strings.Split(contentURL[5:], "/")
	if len(parts) < 2 {
		return semantic.ReturnActionError(c, action, "invalid s3 URL format", nil)
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
		return semantic.ReturnActionError(c, action, "data not found", err)
	}
	defer func() {
		if err := result.Body.Close(); err != nil {
			log.Printf("Failed to close S3 response body: %v", err)
		}
	}()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return semantic.ReturnActionError(c, action, "failed to read data", err)
	}

	contentType := "application/json"
	if result.ContentType != nil {
		contentType = *result.ContentType
	}

	log.Printf("Fetched workflow result via semantic action: %s (size: %d bytes)", key, len(data))

	// Check if result should be written to file
	var outputFile string

	// Check additionalProperty for outputFile
	if action.Properties != nil {
		if of, ok := action.Properties["outputFile"].(string); ok {
			outputFile = of
			log.Printf("DEBUG: Found outputFile in Properties: %s", outputFile)
		}
	}

	// Check for outputType in Properties
	outputType := "inline"
	if action.Properties != nil {
		if ot, ok := action.Properties["outputType"].(string); ok {
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
			return semantic.ReturnActionError(c, action, "Failed to create output directory", err)
		}

		// Write result to file
		if err := os.WriteFile(outputFile, data, 0644); err != nil {
			return semantic.ReturnActionError(c, action, "Failed to write result to file", err)
		}

		log.Printf("Wrote workflow result to file: %s", outputFile)

		// Use semantic Result structure for file output
		action.Result = &semantic.SemanticResult{
			Type:   "DigitalDocument",
			Format: contentType,
			Value: map[string]interface{}{
				"contentUrl":     outputFile,
				"encodingFormat": contentType,
				"contentSize":    int64(len(data)),
			},
		}
	} else {
		// Return inline result
		action.Result = &semantic.SemanticResult{
			Type:   "Dataset",
			Format: contentType,
			Output: string(data),
			Value: map[string]interface{}{
				"contentSize": int64(len(data)),
			},
		}
	}

	semantic.SetSuccessOnAction(action)
	return c.JSON(http.StatusOK, action)
}

// handleSemanticStore wraps the implementation to match ActionHandler signature
func handleSemanticStore(c echo.Context, actionInterface interface{}) error {
	action, ok := actionInterface.(*semantic.SemanticAction)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid action type")
	}
	return handleSemanticStoreImpl(c, action)
}

// handleSemanticRetrieve wraps the implementation to match ActionHandler signature
func handleSemanticRetrieve(c echo.Context, actionInterface interface{}) error {
	action, ok := actionInterface.(*semantic.SemanticAction)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid action type")
	}
	return handleSemanticRetrieveImpl(c, action)
}
