package response

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	pkgerrors "github.com/possibities/gin-core/pkg/errors"
	pkgi18n "github.com/possibities/gin-core/pkg/i18n"
	pkglogger "github.com/possibities/gin-core/pkg/logger"
	"go.uber.org/zap"
)

type Body struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
	TraceID string `json:"trace_id"`
}

func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{
		Code:    0,
		Message: pkgi18n.Localize(c, "response.success", nil, "success"),
		Data:    data,
		TraceID: traceID(c),
	})
}

func Fail(c *gin.Context, err error) {
	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		FailWithStatus(c, pkgerrors.ErrInvalidRequest.HTTPCode, pkgerrors.ErrInvalidRequest.Code, pkgi18n.LocalizeValidationErrors(c, validationErrs))
		return
	}
	if isClientBindError(err) {
		FailWithStatus(c, pkgerrors.ErrInvalidRequest.HTTPCode, pkgerrors.ErrInvalidRequest.Code, pkgi18n.LocalizeBizError(c, pkgerrors.ErrInvalidRequest))
		return
	}

	var bizErr *pkgerrors.BizError
	if errors.As(err, &bizErr) {
		FailWithStatus(c, bizErr.HTTPCode, bizErr.Code, pkgi18n.LocalizeBizError(c, bizErr))
		return
	}
	pkglogger.FromContext(requestContext(c)).Error("unhandled error", zap.Error(err))
	FailWithStatus(c, pkgerrors.ErrInternal.HTTPCode, pkgerrors.ErrInternal.Code, pkgi18n.LocalizeBizError(c, pkgerrors.ErrInternal))
}

func FailWithStatus(c *gin.Context, statusCode, code int, message string) {
	c.JSON(statusCode, Body{Code: code, Message: message, Data: nil, TraceID: traceID(c)})
}

func traceID(c *gin.Context) string {
	if traceID, ok := c.Get("trace_id"); ok {
		if s, ok := traceID.(string); ok {
			return s
		}
	}
	return pkglogger.TraceIDFromContext(requestContext(c))
}

func requestContext(c *gin.Context) context.Context {
	if c != nil && c.Request != nil {
		return c.Request.Context()
	}
	return context.Background()
}

func isClientBindError(err error) bool {
	if err == nil {
		return false
	}

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return true
	}

	var unmarshalTypeErr *json.UnmarshalTypeError
	if errors.As(err, &unmarshalTypeErr) {
		return true
	}

	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}
