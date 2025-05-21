package ctxkeys

type contextKey string

const (
	TraceIDKey   contextKey = "trace_id"
	RequestIDKey contextKey = "request_id"
	UserIDKey    contextKey = "user_id"
	RolesKey     contextKey = "roles"
	IPAddressKey contextKey = "ip_address"
	UserAgentKey contextKey = "user_agent"
	JTI          contextKey = "jti"
)
