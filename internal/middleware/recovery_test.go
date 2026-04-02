package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/possibities/gin-core/pkg/response"
)

func TestRecoveryReturnsUnifiedErrorBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestID(), Recovery())
	router.GET("/panic", func(_ *gin.Context) {
		panic("boom")
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", recorder.Code)
	}

	var body response.Body
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body.Code != 50001 {
		t.Fatalf("expected internal error code, got %d", body.Code)
	}
	if body.TraceID == "" {
		t.Fatal("expected trace_id in recovery response")
	}
}
