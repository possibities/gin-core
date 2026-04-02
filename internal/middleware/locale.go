package middleware

import (
	"github.com/gin-gonic/gin"
	pkgi18n "github.com/possibities/gin-boilerplate/pkg/i18n"
)

func Locale(translator *pkgi18n.Translator) gin.HandlerFunc {
	return func(c *gin.Context) {
		localizer, locale := translator.LocalizerForHeader(c.GetHeader("Accept-Language"))
		c.Set(pkgi18n.LocalizerKey, localizer)
		c.Set(pkgi18n.LocaleKey, locale)
		c.Request = c.Request.WithContext(pkgi18n.WithRequestContext(c.Request.Context(), localizer, locale))
		c.Writer.Header().Set("Content-Language", locale)
		c.Next()
	}
}
