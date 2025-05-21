// analytics-api/internal/transport/grps/handler.go

package grpc

import (
	"context"
	"io"

	"github.com/YaganovValera/analytics-system/common/ctxkeys"
	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"
	commonpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/common"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/analytics-api/internal/usecase"

	"go.opentelemetry.io/otel"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	analyticspb.UnimplementedAnalyticsServiceServer
	getHandler       usecase.GetCandlesHandler
	streamHandler    usecase.StreamCandlesHandler
	subscribeHandler usecase.SubscribeCandlesHandler
}

func NewServer(get usecase.GetCandlesHandler, stream usecase.StreamCandlesHandler, sub usecase.SubscribeCandlesHandler) *Server {
	return &Server{
		getHandler:       get,
		streamHandler:    stream,
		subscribeHandler: sub,
	}
}

func (s *Server) GetCandles(ctx context.Context, req *analyticspb.QueryCandlesRequest) (*analyticspb.GetCandlesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is nil")
	}

	ctx, span := otel.Tracer("analytics-api/grpc").Start(ctx, "GetCandles")
	defer span.End()
	metrics.GRPCRequestsTotal.WithLabelValues("GetCandles").Inc()

	ctx = enrichContextWithMetadata(ctx, req.Metadata)

	if req.Symbol == "" || req.Interval == commonpb.AggregationInterval_AGG_INTERVAL_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "invalid symbol or interval")
	}

	resp, err := s.getHandler.Handle(ctx, req)
	if err != nil {
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "query failed: %v", err)
	}
	return resp, nil
}

func (s *Server) StreamCandles(req *analyticspb.StreamCandlesRequest, stream analyticspb.AnalyticsService_StreamCandlesServer) error {
	if req == nil {
		return status.Error(codes.InvalidArgument, "request is nil")
	}

	ctx, span := otel.Tracer("analytics-api/grpc").Start(stream.Context(), "StreamCandles")
	defer span.End()
	metrics.GRPCRequestsTotal.WithLabelValues("StreamCandles").Inc()

	ctx = enrichContextWithMetadata(ctx, req.Metadata)

	if req.Symbol == "" || req.Interval == commonpb.AggregationInterval_AGG_INTERVAL_UNSPECIFIED {
		return status.Error(codes.InvalidArgument, "invalid symbol or interval")
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

func (s *Server) SubscribeCandles(stream analyticspb.AnalyticsService_SubscribeCandlesServer) error {
	ctx, span := otel.Tracer("analytics-api/grpc").Start(stream.Context(), "SubscribeCandles")
	defer span.End()
	metrics.GRPCRequestsTotal.WithLabelValues("SubscribeCandles").Inc()

	reqChan := make(chan *analyticspb.CandleStreamRequest, 100)
	respChan, err := s.subscribeHandler.Handle(ctx, reqChan)
	if err != nil {
		span.RecordError(err)
		return status.Errorf(codes.Internal, "subscribe handler error: %v", err)
	}

	// Чтение от клиента (subscribe / ack)
	go func() {
		defer close(reqChan)
		for {
			req, err := stream.Recv()
			if err != nil {
				if err == io.EOF || context.Canceled == ctx.Err() {
					return
				}
				// При ошибке прерываем поток
				return
			}
			reqChan <- req
		}
	}()

	// Отправка событий клиенту
	for {
		select {
		case <-ctx.Done():
			return nil
		case resp, ok := <-respChan:
			if !ok {
				return nil
			}
			if err := stream.Send(resp); err != nil {
				return status.Errorf(codes.Unavailable, "send error: %v", err)
			}
		}
	}
}

func enrichContextWithMetadata(ctx context.Context, meta *commonpb.RequestMetadata) context.Context {
	if meta == nil {
		return ctx
	}
	if meta.TraceId != "" {
		ctx = context.WithValue(ctx, ctxkeys.TraceIDKey, meta.TraceId)
	}
	if meta.IpAddress != "" {
		ctx = context.WithValue(ctx, ctxkeys.IPAddressKey, meta.IpAddress)
	}
	if meta.UserAgent != "" {
		ctx = context.WithValue(ctx, ctxkeys.UserAgentKey, meta.UserAgent)
	}
	return ctx
}
