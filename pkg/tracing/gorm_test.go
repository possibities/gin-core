package tracing

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/possibities/gin-core/pkg/metrics"
	"gorm.io/gorm"
)

type gormMetricsUser struct {
	ID   uint
	Name string
}

func (gormMetricsUser) TableName() string {
	return "users"
}

func TestRegisterGORMCallbacksRecordsDBQueryMetrics(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}

	registry := metrics.New(nil)
	if err := RegisterGORMCallbacks(db, &Provider{}, registry); err != nil {
		t.Fatalf("register callbacks: %v", err)
	}
	if err := db.AutoMigrate(&gormMetricsUser{}); err != nil {
		t.Fatalf("migrate schema: %v", err)
	}
	if err := db.Create(&gormMetricsUser{Name: "alice"}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	var user gormMetricsUser
	if err := db.Where("name = ?", "alice").First(&user).Error; err != nil {
		t.Fatalf("query user: %v", err)
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	registry.Handler().ServeHTTP(recorder, req)

	body := recorder.Body.String()
	checks := []string{
		"db_query_duration_seconds_count{operation=\"create\",table=\"users\"} ",
		"db_query_duration_seconds_count{operation=\"query\",table=\"users\"} ",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Fatalf("expected metrics output to contain %q", want)
		}
	}
}
