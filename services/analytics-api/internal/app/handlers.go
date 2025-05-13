// services/analytics-api/internal/app/handlers.go
package app

import (
	"context"

	"github.com/YaganovValera/analytics-system/common/logger"
	analyticspb "github.com/YaganovValera/analytics-system/proto/v1/analytics"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/storage"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Handler теперь встраивает UnimplementedAnalyticsServiceServer,
// чтобы автоматически реализовать mustEmbedUnimplementedAnalyticsServiceServer().
type Handler struct {
	analyticspb.UnimplementedAnalyticsServiceServer
	repo storage.Repository
	log  *logger.Logger
}

// NewHandler возвращает новый экземпляр Handler.
func NewHandler(repo storage.Repository, log *logger.Logger) *Handler {
	return &Handler{
		repo: repo,
		log:  log.Named("handler"),
	}
}

// GetCandles реализует RPC GetCandles.
func (h *Handler) GetCandles(
	ctx context.Context,
	req *analyticspb.GetCandlesRequest,
) (*analyticspb.GetCandlesResponse, error) {
	metrics.GetCandlesRequests.Inc()

	start := req.Start.AsTime().UTC()
	end := req.End.AsTime().UTC()
	candles, nextToken, err := h.repo.GetCandles(
		ctx,
		req.Symbol,
		start,
		end,
		req.Interval,
		int(req.Pagination.PageSize),
		req.Pagination.PageToken,
	)
	if err != nil {
		metrics.GetCandlesErrors.Inc()
		h.log.WithContext(ctx).Error("repo.GetCandles failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get candles: %v", err)
	}

	return &analyticspb.GetCandlesResponse{
		Candles:       candles,
		NextPageToken: nextToken,
	}, nil
}

// StreamCandles реализует RPC StreamCandles.
func (h *Handler) StreamCandles(
	req *analyticspb.GetCandlesRequest,
	srv analyticspb.AnalyticsService_StreamCandlesServer,
) error {
	metrics.StreamCandlesRequests.Inc()

	start := req.Start.AsTime().UTC()
	end := req.End.AsTime().UTC()
	events, err := h.repo.StreamCandles(srv.Context(), req.Symbol, start, end, req.Interval)
	if err != nil {
		metrics.StreamCandlesErrors.Inc()
		h.log.WithContext(srv.Context()).Error("repo.StreamCandles failed", zap.Error(err))
		return status.Errorf(codes.Internal, "failed to stream candles: %v", err)
	}

	for ev := range events {
		// счётчики событий/ошибок
		if _, isErr := ev.Payload.(*analyticspb.CandleEvent_StreamError); isErr {
			metrics.StreamCandlesErrors.Inc()
		} else {
			metrics.StreamCandlesEvents.Inc()
		}

		if sendErr := srv.Send(ev); sendErr != nil {
			h.log.WithContext(srv.Context()).Error("send to gRPC stream failed", zap.Error(sendErr))
			return status.Errorf(codes.Internal, "stream send error: %v", sendErr)
		}
	}

	return nil
}
