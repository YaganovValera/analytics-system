// api-gateway/internal/client/auth/client.go
package auth

import (
	"context"

	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"
	"google.golang.org/grpc"
)

// Client оборачивает gRPC-клиент AuthService.
type Client struct {
	authpb.AuthServiceClient
}

// New создаёт клиента AuthService.
func New(conn *grpc.ClientConn) *Client {
	return &Client{
		AuthServiceClient: authpb.NewAuthServiceClient(conn),
	}
}

// Ping проверяет доступность AuthService через ValidateToken с фейковым токеном.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.ValidateToken(ctx, &authpb.ValidateTokenRequest{Token: "__ping__"})
	return err
}
