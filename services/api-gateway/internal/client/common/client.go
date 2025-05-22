// api-gateway/internal/client/common/client.go
package common

import (
	commonpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/common"

	"google.golang.org/grpc"
)

// Client оборачивает gRPC-клиент CommonService.
type Client struct {
	commonpb.CommonServiceClient
}

// New создаёт клиента CommonService.
func New(conn *grpc.ClientConn) *Client {
	return &Client{
		CommonServiceClient: commonpb.NewCommonServiceClient(conn),
	}
}
