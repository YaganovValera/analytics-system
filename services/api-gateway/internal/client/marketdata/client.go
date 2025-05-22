// api-gateway/internal/client/marketdata/client.go

package marketdata

import (
	mdpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/marketdata"
	"google.golang.org/grpc"
)

type Client struct {
	mdpb.MarketDataServiceClient
}

func New(conn *grpc.ClientConn) *Client {
	return &Client{
		MarketDataServiceClient: mdpb.NewMarketDataServiceClient(conn),
	}
}
