// api-gateway/internal/handler/handler.go
package handler

import (
	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"
	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"
	commonpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/common"
	mdpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/marketdata"
)

// Handler агрегирует все зависимости для HTTP хендлеров.
type Handler struct {
	Auth       authpb.AuthServiceClient
	Analytics  analyticspb.AnalyticsServiceClient
	Common     commonpb.CommonServiceClient
	MarketData mdpb.MarketDataServiceClient
}

func NewHandler(
	auth authpb.AuthServiceClient,
	analytics analyticspb.AnalyticsServiceClient,
	common commonpb.CommonServiceClient,
	md mdpb.MarketDataServiceClient,
) *Handler {
	return &Handler{
		Auth:       auth,
		Analytics:  analytics,
		Common:     common,
		MarketData: md,
	}
}
