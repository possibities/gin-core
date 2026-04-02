package pkgerrors

import "net/http"

type BizError struct {
	HTTPCode  int    `json:"-"`
	Code      int    `json:"code"`
	MessageID string `json:"-"`
	Message   string `json:"message"`
}

func (e *BizError) Error() string {
	return e.Message
}

var (
	ErrNotFound            = &BizError{HTTPCode: http.StatusNotFound, Code: 10001, MessageID: "errors.common.not_found", Message: "resource not found"}
	ErrUnauthorized        = &BizError{HTTPCode: http.StatusUnauthorized, Code: 10002, MessageID: "errors.common.unauthorized", Message: "unauthorized"}
	ErrTokenInvalid        = &BizError{HTTPCode: http.StatusUnauthorized, Code: 10003, MessageID: "errors.common.token_invalid", Message: "invalid or expired token"}
	ErrTokenBlacklisted    = &BizError{HTTPCode: http.StatusUnauthorized, Code: 10004, MessageID: "errors.common.token_blacklisted", Message: "token has been revoked"}
	ErrForbidden           = &BizError{HTTPCode: http.StatusForbidden, Code: 10005, MessageID: "errors.common.forbidden", Message: "forbidden"}
	ErrServiceShuttingDown = &BizError{HTTPCode: http.StatusServiceUnavailable, Code: 10006, MessageID: "errors.common.shutting_down", Message: "service is shutting down"}
	ErrDependencyNotReady  = &BizError{HTTPCode: http.StatusServiceUnavailable, Code: 10007, MessageID: "errors.common.dependency_not_ready", Message: "dependencies are not ready"}
	ErrTooManyRequests     = &BizError{HTTPCode: http.StatusTooManyRequests, Code: 10008, MessageID: "errors.common.too_many_requests", Message: "too many requests"}
	ErrInvalidRequest      = &BizError{HTTPCode: http.StatusBadRequest, Code: 10009, MessageID: "errors.common.invalid_request", Message: "invalid request"}
	ErrUserEmailExists     = &BizError{HTTPCode: http.StatusConflict, Code: 10101, MessageID: "errors.user.email_exists", Message: "email already exists"}
	ErrInternal            = &BizError{HTTPCode: http.StatusInternalServerError, Code: 50001, MessageID: "errors.common.internal", Message: "internal server error"}
)
