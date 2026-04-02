package i18n

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"text/template"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/possibities/gin-boilerplate/locales"
	pkgerrors "github.com/possibities/gin-boilerplate/pkg/errors"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

const (
	LocalizerKey = "localizer"
	LocaleKey    = "locale"
)

type translatorContextKey string

const (
	localizerContextKey translatorContextKey = "i18n_localizer"
	localeContextKey    translatorContextKey = "i18n_locale"
)

var validatorTagNameOnce sync.Once

type Translator struct {
	bundle     *goi18n.Bundle
	matcher    language.Matcher
	defaultTag language.Tag
}

func New() (*Translator, error) {
	bundle := goi18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("yaml", yaml.Unmarshal)

	for _, name := range []string{"en.yaml", "zh-CN.yaml"} {
		data, err := locales.FS.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read locale file %s: %w", name, err)
		}
		if _, err := bundle.ParseMessageFileBytes(data, name); err != nil {
			return nil, fmt.Errorf("parse locale file %s: %w", name, err)
		}
	}

	registerValidationTagNames()

	return &Translator{
		bundle:     bundle,
		matcher:    language.NewMatcher([]language.Tag{language.English, language.MustParse("zh-CN")}),
		defaultTag: language.English,
	}, nil
}

func (t *Translator) LocalizerForHeader(header string) (*goi18n.Localizer, string) {
	tags, _, err := language.ParseAcceptLanguage(header)
	if err != nil || len(tags) == 0 {
		return t.defaultLocalizer(), t.defaultTag.String()
	}

	tag, _, _ := t.matcher.Match(tags...)
	return goi18n.NewLocalizer(t.bundle, tag.String(), t.defaultTag.String()), tag.String()
}

func WithRequestContext(ctx context.Context, localizer *goi18n.Localizer, locale string) context.Context {
	ctx = context.WithValue(ctx, localizerContextKey, localizer)
	return context.WithValue(ctx, localeContextKey, locale)
}

func LocalizerFromContext(ctx context.Context) *goi18n.Localizer {
	if ctx == nil {
		return nil
	}
	localizer, _ := ctx.Value(localizerContextKey).(*goi18n.Localizer)
	return localizer
}

func LocaleFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	locale, _ := ctx.Value(localeContextKey).(string)
	return locale
}

func Localize(c *gin.Context, messageID string, templateData map[string]any, defaultMessage string) string {
	localizer := localizerFromGin(c)
	if localizer == nil {
		return renderFallback(defaultMessage, templateData)
	}

	message, err := localizer.Localize(&goi18n.LocalizeConfig{
		MessageID:      messageID,
		TemplateData:   templateData,
		DefaultMessage: &goi18n.Message{ID: messageID, Other: defaultMessage},
	})
	if err != nil {
		return renderFallback(defaultMessage, templateData)
	}
	return message
}

func LocalizeBizError(c *gin.Context, bizErr *pkgerrors.BizError) string {
	if bizErr == nil {
		return ""
	}
	return Localize(c, bizErr.MessageID, nil, bizErr.Message)
}

func LocalizeValidationErrors(c *gin.Context, errs validator.ValidationErrors) string {
	if len(errs) == 0 {
		return Localize(c, pkgerrors.ErrInvalidRequest.MessageID, nil, pkgerrors.ErrInvalidRequest.Message)
	}

	issue := errs[0]
	field := normalizeFieldName(issue.Field())
	if field == "" {
		field = "field"
	}

	templateData := map[string]any{"Field": field}

	switch issue.Tag() {
	case "required":
		return Localize(c, "validation.required", templateData, "{{.Field}} is required")
	case "email":
		return Localize(c, "validation.email", templateData, "{{.Field}} must be a valid email address")
	case "max":
		templateData["Max"] = issue.Param()
		return Localize(c, "validation.max", templateData, "{{.Field}} must be at most {{.Max}} characters")
	default:
		return Localize(c, "validation.invalid", templateData, "{{.Field}} is invalid")
	}
}

func (t *Translator) defaultLocalizer() *goi18n.Localizer {
	return goi18n.NewLocalizer(t.bundle, t.defaultTag.String())
}

func localizerFromGin(c *gin.Context) *goi18n.Localizer {
	if c == nil {
		return nil
	}
	if localizer, ok := c.Get(LocalizerKey); ok {
		if localized, ok := localizer.(*goi18n.Localizer); ok {
			return localized
		}
	}
	if c.Request != nil {
		return LocalizerFromContext(c.Request.Context())
	}
	return nil
}

func renderFallback(message string, data map[string]any) string {
	if message == "" || len(data) == 0 {
		return message
	}
	tpl, err := template.New("fallback").Parse(message)
	if err != nil {
		return message
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return message
	}
	return buf.String()
}

func registerValidationTagNames() {
	validatorTagNameOnce.Do(func() {
		engine, ok := binding.Validator.Engine().(*validator.Validate)
		if !ok {
			return
		}
		engine.RegisterTagNameFunc(func(field reflect.StructField) string {
			name := field.Tag.Get("json")
			if name == "" {
				return field.Name
			}
			name = strings.Split(name, ",")[0]
			if name == "" || name == "-" {
				return field.Name
			}
			return name
		})
	})
}

func normalizeFieldName(field string) string {
	field = strings.TrimSpace(field)
	if field == "" {
		return ""
	}
	if strings.Contains(field, "_") {
		return strings.ToLower(field)
	}

	runes := []rune(field)
	var builder strings.Builder
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 && shouldInsertUnderscore(runes, i) {
				builder.WriteByte('_')
			}
			builder.WriteRune(unicode.ToLower(r))
			continue
		}
		builder.WriteRune(unicode.ToLower(r))
	}
	return builder.String()
}

func shouldInsertUnderscore(runes []rune, index int) bool {
	prev := runes[index-1]
	if unicode.IsLower(prev) || unicode.IsDigit(prev) {
		return true
	}
	if index+1 >= len(runes) {
		return false
	}
	next := runes[index+1]
	return unicode.IsLower(next)
}
