package handler

var (
	_ = healthStatusDoc{}
	_ = sessionDoc{}
	_ = userProfileDoc{}
	_ = successResponseHealth{}
	_ = successResponseSession{}
	_ = successResponseUserProfile{}
	_ = errorResponse{}
)

type healthStatusDoc struct {
	Status string `json:"status"`
}

type sessionDoc struct {
	UserID   uint   `json:"user_id"`
	Role     string `json:"role"`
	TenantID string `json:"tenant_id"`
	Scope    string `json:"scope,omitempty"`
}

type userProfileDoc struct {
	ID        uint   `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	TenantID  string `json:"tenant_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type successResponseHealth struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    healthStatusDoc `json:"data"`
	TraceID string          `json:"trace_id"`
}

type successResponseSession struct {
	Code    int        `json:"code"`
	Message string     `json:"message"`
	Data    sessionDoc `json:"data"`
	TraceID string     `json:"trace_id"`
}

type successResponseUserProfile struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    userProfileDoc `json:"data"`
	TraceID string         `json:"trace_id"`
}

type errorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
	TraceID string `json:"trace_id"`
}
