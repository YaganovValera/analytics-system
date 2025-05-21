// api-gateway/internal/client/analytics/client.go
package analyticsclient

import (
	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"
	"google.golang.org/grpc"
)

type Client struct {
	analyticspb.AnalyticsServiceClient
}

func New(conn *grpc.ClientConn) *Client {
	return &Client{
		AnalyticsServiceClient: analyticspb.NewAnalyticsServiceClient(conn),
	}
}
