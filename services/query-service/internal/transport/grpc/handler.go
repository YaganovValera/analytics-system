// query-service/internal/transport/grpc/handler.go
package grpc

import (
	"context"

	querypb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/query"
	"github.com/YaganovValera/analytics-system/services/query-service/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/query-service/internal/usecase"

	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type server struct {
	querypb.UnimplementedQueryServiceServer
	exec usecase.Executor
	log  *zap.Logger
}

func NewServer(exec usecase.Executor) querypb.QueryServiceServer {
	return &server{
		exec: exec,
		log:  zap.L().Named("grpc"),
	}
}

func (s *server) ExecuteSQL(ctx context.Context, req *querypb.ExecuteSQLRequest) (*querypb.ExecuteSQLResponse, error) {
	ctx, span := otel.Tracer("query/grpc").Start(ctx, "ExecuteSQL")
	defer span.End()

	metrics.GRPCRequestsTotal.WithLabelValues("ExecuteSQL").Inc()

	if req == nil || req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}

	columns, rows, err := s.exec.Execute(ctx, req.Query, req.Parameters)
	if err != nil {
		s.log.Warn("ExecuteSQL failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "execution failed: %v", err)
	}

	resp := &querypb.ExecuteSQLResponse{
		Columns: columns,
	}
	for _, row := range rows {
		resp.Rows = append(resp.Rows, &querypb.Row{Values: row})
	}

	return resp, nil
}
