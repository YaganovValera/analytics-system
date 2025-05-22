// analytics-api/internal/usecase/get_symbols.go
package usecase

import (
	"context"

	commonpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/common"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/storage/timescaledb"
)

type GetSymbolsHandler interface {
	Handle(ctx context.Context, req *commonpb.ListSymbolsRequest) (*commonpb.ListSymbolsResponse, error)
}

type getSymbolsHandler struct {
	repo timescaledb.Repository
}

func NewGetSymbolsHandler(repo timescaledb.Repository) GetSymbolsHandler {
	return &getSymbolsHandler{repo: repo}
}

func (h *getSymbolsHandler) Handle(ctx context.Context, req *commonpb.ListSymbolsRequest) (*commonpb.ListSymbolsResponse, error) {
	symbols, next, err := h.repo.ListSymbols(ctx, req.GetPagination())
	if err != nil {
		return nil, err
	}
	return &commonpb.ListSymbolsResponse{
		Symbols:       symbols,
		NextPageToken: next,
	}, nil
}
