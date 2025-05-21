// api-gateway/internal/handler/handler.go
package handler

import (
	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"
	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"
)

// Handler агрегирует все зависимости для HTTP хендлеров.
type Handler struct {
	Auth      authpb.AuthServiceClient
	Analytics analyticspb.AnalyticsServiceClient
}

// NewHandler создаёт Handler.
func NewHandler(auth authpb.AuthServiceClient, analytics analyticspb.AnalyticsServiceClient) *Handler {
	return &Handler{
		Auth:      auth,
		Analytics: analytics,
	}
}
