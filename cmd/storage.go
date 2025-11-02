package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/labstack/echo/v4"
)

var s3Client *s3.Client

func init() {
	// Initialize S3 client
	accessKey := os.Getenv("HETZNER_S3_ACCESS_KEY")
	secretKey := os.Getenv("HETZNER_S3_SECRET_KEY")
	endpoint := os.Getenv("HETZNER_S3_URL")

	if accessKey == "" || secretKey == "" || endpoint == "" {
		log.Fatal("Missing S3 credentials: HETZNER_S3_ACCESS_KEY, HETZNER_S3_SECRET_KEY, HETZNER_S3_URL")
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithRegion("fsn1"),
	)
	if err != nil {
		log.Fatalf("Failed to load S3 config: %v", err)
	}

	s3Client = s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	log.Println("S3 client initialized successfully")
}

// StoreRequest represents a request to store data
type StoreRequest struct {
	WorkflowID string `json:"workflowId"`
	ActionID   string `json:"actionId"`
	Data       string `json:"data"`
	Format     string `json:"format,omitempty"` // application/json, text/plain, etc.
}

// StoreResponse returns the reference to stored data
type StoreResponse struct {
	Type           string `json:"@type"`
	ID             string `json:"@id"`
	ContentURL     string `json:"contentUrl"`
	EncodingFormat string `json:"encodingFormat"`
	ContentSize    int64  `json:"contentSize"`
}

// FetchResponse returns the fetched data
type FetchResponse struct {
	Data           string `json:"data"`
	EncodingFormat string `json:"encodingFormat"`
	ContentSize    int64  `json:"contentSize"`
}

func handleStore(c echo.Context) error {
	var req StoreRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if req.WorkflowID == "" || req.ActionID == "" || req.Data == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "workflowId, actionId, and data are required"})
	}

	if req.Format == "" {
		req.Format = "application/json"
	}

	// Generate S3 key: workflow-results/{workflowId}/{actionId}.json
	bucket := os.Getenv("HETZNER_S3_BUCKET")
	if bucket == "" {
		bucket = "px-semantic"
	}

	key := fmt.Sprintf("workflow-results/%s/%s.json", req.WorkflowID, req.ActionID)

	// Upload to S3
	dataBytes := []byte(req.Data)
	_, err := s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(dataBytes),
		ContentType: aws.String(req.Format),
	})
	if err != nil {
		log.Printf("Failed to upload to S3: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to store data"})
	}

	log.Printf("Stored workflow result: %s (size: %d bytes)", key, len(dataBytes))

	// Return semantic reference
	response := StoreResponse{
		Type:           "DataDownload",
		ID:             fmt.Sprintf("#%s-result", req.ActionID),
		ContentURL:     fmt.Sprintf("s3://%s/%s", bucket, key),
		EncodingFormat: req.Format,
		ContentSize:    int64(len(dataBytes)),
	}

	return c.JSON(http.StatusOK, response)
}

func handleFetch(c echo.Context) error {
	key := c.Param("key")
	if key == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "key is required"})
	}

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

	response := FetchResponse{
		Data:           string(data),
		EncodingFormat: contentType,
		ContentSize:    int64(len(data)),
	}

	log.Printf("Fetched workflow result: %s (size: %d bytes)", key, len(data))

	return c.JSON(http.StatusOK, response)
}
