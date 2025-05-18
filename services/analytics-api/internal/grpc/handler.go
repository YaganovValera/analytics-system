// github.com/YaganovValera/analytics-system/services/analytics-api/internal/grpc/handler.go
// internal/grpc/handler.go
package grpc

import (
	"context"

	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/metrics"
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	analyticspb.UnimplementedAnalyticsServiceServer
	getHandler    GetCandlesHandler
	streamHandler StreamCandlesHandler
}

func NewServer(get GetCandlesHandler, stream StreamCandlesHandler) *Server {
	return &Server{
		getHandler:    get,
		streamHandler: stream,
	}
}

// GetCandles handles unary request to fetch historical candles.
func (s *Server) GetCandles(ctx context.Context, req *analyticspb.GetCandlesRequest) (*analyticspb.GetCandlesResponse, error) {
	ctx, span := otel.Tracer("analytics-api/grpc").Start(ctx, "GetCandles")
	defer span.End()
	metrics.GRPCRequestsTotal.WithLabelValues("GetCandles").Inc()

	if req == nil || req.Symbol == "" || req.Interval == 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	resp, err := s.getHandler.Handle(ctx, req)
	if err != nil {
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "query failed: %v", err)
	}
	return resp, nil
}

// StreamCandles streams candle events from Kafka to client.
func (s *Server) StreamCandles(req *analyticspb.GetCandlesRequest, stream analyticspb.AnalyticsService_StreamCandlesServer) error {
	ctx, span := otel.Tracer("analytics-api/grpc").Start(stream.Context(), "StreamCandles")
	defer span.End()
	metrics.GRPCRequestsTotal.WithLabelValues("StreamCandles").Inc()

	if req == nil || req.Symbol == "" || req.Interval == 0 {
		return status.Error(codes.InvalidArgument, "invalid request")
	}

	ch, err := s.streamHandler.Handle(ctx, req)
	if err != nil {
		span.RecordError(err)
		return status.Errorf(codes.Internal, "stream handler error: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-ch:
			if !ok {
				return nil
			}
			metrics.StreamEventsTotal.WithLabelValues(req.Interval.String()).Inc()
			if err := stream.Send(evt); err != nil {
				return status.Errorf(codes.Unavailable, "send error: %v", err)
			}
		}
	}
}

// Handler interfaces to be implemented in usecase.
type GetCandlesHandler interface {
	Handle(ctx context.Context, req *analyticspb.GetCandlesRequest) (*analyticspb.GetCandlesResponse, error)
}

type StreamCandlesHandler interface {
	Handle(ctx context.Context, req *analyticspb.GetCandlesRequest) (<-chan *analyticspb.CandleEvent, error)
}
