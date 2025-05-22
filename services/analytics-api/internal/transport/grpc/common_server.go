// analytics-api/internal/transport/grps/common_server.go
package grpc

import (
	"context"

	commonpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/common"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/usecase"
)

type commonServer struct {
	commonpb.UnimplementedCommonServiceServer
	getSymbols usecase.GetSymbolsHandler
}

func NewCommonServer(getSymbols usecase.GetSymbolsHandler) commonpb.CommonServiceServer {
	return &commonServer{getSymbols: getSymbols}
}

func (s *commonServer) ListSymbols(ctx context.Context, req *commonpb.ListSymbolsRequest) (*commonpb.ListSymbolsResponse, error) {
	return s.getSymbols.Handle(ctx, req)
}
