package i18n

import (
	"testing"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
)

func TestTranslatorMatchesChineseLocale(t *testing.T) {
	translator, err := New()
	if err != nil {
		t.Fatalf("new translator: %v", err)
	}

	localizer, locale := translator.LocalizerForHeader("zh-CN,zh;q=0.9,en;q=0.8")
	if locale != "zh-CN" {
		t.Fatalf("expected zh-CN locale, got %q", locale)
	}

	message, err := localizer.Localize(&goi18n.LocalizeConfig{
		MessageID:      "errors.common.unauthorized",
		DefaultMessage: &goi18n.Message{ID: "errors.common.unauthorized", Other: "unauthorized"},
	})
	if err != nil {
		t.Fatalf("localize message: %v", err)
	}
	if message != "未授权" {
		t.Fatalf("expected chinese message, got %q", message)
	}
}

func TestNormalizeFieldNameHandlesInitialisms(t *testing.T) {
	tests := map[string]string{
		"Name":     "name",
		"TenantID": "tenant_id",
		"URLPath":  "url_path",
	}

	for input, want := range tests {
		if got := normalizeFieldName(input); got != want {
			t.Fatalf("normalizeFieldName(%q) = %q, want %q", input, got, want)
		}
	}
}
