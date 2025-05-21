// api-gateway/internal/client/auth/client.go
package authclient

import (
	"context"

	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"
	"google.golang.org/grpc"
)

type Client struct {
	authpb.AuthServiceClient
}

func New(conn *grpc.ClientConn) *Client {
	return &Client{
		AuthServiceClient: authpb.NewAuthServiceClient(conn),
	}
}

// Ping проверяет доступность AuthService через ValidateToken.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.ValidateToken(ctx, &authpb.ValidateTokenRequest{Token: "fake"})
	return err // ожидаем ошибку, главное — соединение живое
}
