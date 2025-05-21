// api-gateway/internal/client/analytics/client.go
package analytics

import (
	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"
	"google.golang.org/grpc"
)

// Client оборачивает gRPC-клиент AnalyticsService.
type Client struct {
	analyticspb.AnalyticsServiceClient
}

// New создаёт клиента AnalyticsService.
func New(conn *grpc.ClientConn) *Client {
	return &Client{
		AnalyticsServiceClient: analyticspb.NewAnalyticsServiceClient(conn),
	}
}
