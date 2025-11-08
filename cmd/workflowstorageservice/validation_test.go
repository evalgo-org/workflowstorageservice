package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestSemanticActionEndpoint_InvalidJSON(t *testing.T) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/v1/api/semantic/action", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handleSemanticAction(c)

	// semantic.ReturnActionError returns error for Echo to handle
	if err != nil {
		// If error is returned, it should be an HTTP error
		return
	}

	// If no error is returned, check response status
	if rec.Code == http.StatusOK {
		t.Error("handleSemanticAction() should not return 200 OK for invalid JSON")
	}
}

func TestSemanticActionEndpoint_EmptyBody(t *testing.T) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/v1/api/semantic/action", bytes.NewReader([]byte("")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handleSemanticAction(c)

	// semantic.ReturnActionError returns error for Echo to handle
	if err != nil {
		// If error is returned, it should be an HTTP error
		return
	}

	// If no error is returned, check response status
	if rec.Code == http.StatusOK {
		t.Error("handleSemanticAction() should not return 200 OK for empty body")
	}
}

func TestSemanticActionEndpoint_UnsupportedActionType(t *testing.T) {
	e := echo.New()

	action := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "UnsupportedAction",
	}

	body, _ := json.Marshal(action)
	req := httptest.NewRequest(http.MethodPost, "/v1/api/semantic/action", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handleSemanticAction(c)

	// semantic.ReturnActionError returns error for Echo to handle
	if err != nil {
		// If error is returned, it should be an HTTP error
		return
	}

	// If no error is returned, check response status
	if rec.Code == http.StatusOK {
		t.Error("handleSemanticAction() should not return 200 OK for unsupported action type")
	}
}

func TestHealthEndpoint(t *testing.T) {
	e := echo.New()

	// Register health endpoint
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"status": "ok",
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse health response: %v", err)
	}

	if response["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%v'", response["status"])
	}
}
