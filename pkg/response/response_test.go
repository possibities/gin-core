package response

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	pkgerrors "github.com/possibities/gin-boilerplate/pkg/errors"
	pkgi18n "github.com/possibities/gin-boilerplate/pkg/i18n"
)

func TestSuccessIncludesTraceID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("trace_id", "trace-123")

	Success(c, gin.H{"ok": true})

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var body Body
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body.Code != 0 || body.Message != "success" || body.TraceID != "trace-123" {
		t.Fatalf("unexpected success body: %+v", body)
	}
}

func TestFailUsesBizError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("trace_id", "trace-456")

	Fail(c, pkgerrors.ErrUnauthorized)

	var body Body
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", recorder.Code)
	}
	if body.Code != pkgerrors.ErrUnauthorized.Code || body.TraceID != "trace-456" {
		t.Fatalf("unexpected error body: %+v", body)
	}
}

func TestFailMapsUnknownErrorsToInternal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("trace_id", "trace-789")

	Fail(c, errors.New("boom"))

	var body Body
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", recorder.Code)
	}
	if body.Code != pkgerrors.ErrInternal.Code || body.Message != pkgerrors.ErrInternal.Message {
		t.Fatalf("unexpected fallback body: %+v", body)
	}
}

func TestFailLocalizesBizErrorWhenLocalizerPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	translator, err := pkgi18n.New()
	if err != nil {
		t.Fatalf("new translator: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	localizer, locale := translator.LocalizerForHeader("zh-CN")
	c.Set(pkgi18n.LocalizerKey, localizer)
	c.Set(pkgi18n.LocaleKey, locale)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(pkgi18n.WithRequestContext(httptest.NewRequest(http.MethodGet, "/", nil).Context(), localizer, locale))

	Fail(c, pkgerrors.ErrUnauthorized)

	var body Body
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body.Message != "未授权" {
		t.Fatalf("expected localized message, got %+v", body)
	}
}
