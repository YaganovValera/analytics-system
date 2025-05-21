// internal/transport/grpc/handler.go
package grpc

import (
	"context"

	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"
	commonpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/common"
	"github.com/YaganovValera/analytics-system/services/auth/internal/jwt"
	"github.com/YaganovValera/analytics-system/services/auth/internal/metrics"
	"github.com/YaganovValera/analytics-system/services/auth/internal/usecase"

	"github.com/YaganovValera/analytics-system/common/ctxkeys"

	"go.opentelemetry.io/otel"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	authpb.UnimplementedAuthServiceServer
	h usecase.Handler
}

func NewServer(handler usecase.Handler) *Server {
	return &Server{h: handler}
}

func (s *Server) Login(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error) {
	ctx, span := otel.Tracer("auth/grpc").Start(ctx, "Login")
	defer span.End()

	metrics.GRPCRequestsTotal.WithLabelValues("Login").Inc()

	if req != nil && req.Metadata != nil {
		ctx = enrichContextWithMetadata(ctx, req.Metadata)
	}

	if req == nil {
		metrics.LoginTotal.WithLabelValues("invalid").Inc()
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	resp, err := s.h.Login.Handle(ctx, req)
	if err != nil {
		metrics.LoginTotal.WithLabelValues("fail").Inc()
		return nil, status.Errorf(codes.Unauthenticated, "login error: %v", err)
	}
	metrics.LoginTotal.WithLabelValues("ok").Inc()
	return resp, nil
}

func (s *Server) RefreshToken(ctx context.Context, req *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error) {
	ctx, span := otel.Tracer("auth/grpc").Start(ctx, "Refresh")
	defer span.End()

	if req == nil || req.RefreshToken == "" {
		metrics.RefreshTotal.WithLabelValues("invalid").Inc()
		return nil, status.Error(codes.InvalidArgument, "empty refresh token")
	}

	resp, err := s.h.Refresh.Handle(ctx, req)
	if err != nil {
		metrics.RefreshTotal.WithLabelValues("fail").Inc()
		return nil, status.Errorf(codes.Unauthenticated, "refresh failed: %v", err)
	}
	metrics.RefreshTotal.WithLabelValues("ok").Inc()
	return resp, nil
}

func (s *Server) ValidateToken(ctx context.Context, req *authpb.ValidateTokenRequest) (*authpb.ValidateTokenResponse, error) {
	ctx, span := otel.Tracer("auth/grpc").Start(ctx, "ValidateToken")
	defer span.End()

	if req == nil || req.Token == "" {
		metrics.ValidateTotal.WithLabelValues("invalid").Inc()
		return nil, status.Error(codes.InvalidArgument, "missing access token")
	}

	resp, err := s.h.Validate.Handle(ctx, req.Token)
	if err != nil {
		metrics.ValidateTotal.WithLabelValues("fail").Inc()
		return nil, status.Errorf(codes.Internal, "validate failed: %v", err)
	}
	metrics.ValidateTotal.WithLabelValues("ok").Inc()
	return resp, nil
}

func (s *Server) RevokeToken(ctx context.Context, req *authpb.RevokeTokenRequest) (*authpb.RevokeTokenResponse, error) {
	ctx, span := otel.Tracer("auth/grpc").Start(ctx, "RevokeToken")
	defer span.End()

	if req == nil || req.Token == "" {
		metrics.RevokeTotal.WithLabelValues("invalid").Inc()
		return nil, status.Error(codes.InvalidArgument, "missing token")
	}

	resp, err := s.h.Revoke.Handle(ctx, req)
	if err != nil {
		metrics.RevokeTotal.WithLabelValues("fail").Inc()
		return nil, status.Errorf(codes.Internal, "revoke failed: %v", err)
	}
	metrics.RevokeTotal.WithLabelValues("ok").Inc()
	return resp, nil
}

func (s *Server) Logout(ctx context.Context, req *authpb.LogoutRequest) (*authpb.LogoutResponse, error) {
	ctx, span := otel.Tracer("auth/grpc").Start(ctx, "Logout")
	defer span.End()

	if req == nil || req.RefreshToken == "" {
		metrics.LogoutTotal.WithLabelValues("invalid").Inc()
		return nil, status.Error(codes.InvalidArgument, "missing refresh token")
	}

	claims, err := jwt.ParseUnverifiedJTI(req.RefreshToken)
	if err != nil {
		metrics.LogoutTotal.WithLabelValues("invalid").Inc()
		return nil, status.Errorf(codes.InvalidArgument, "invalid refresh token: %v", err)
	}

	if err := s.h.Logout.Handle(ctx, claims.JTI); err != nil {
		metrics.LogoutTotal.WithLabelValues("fail").Inc()
		return nil, status.Errorf(codes.Internal, "revoke failed: %v", err)
	}
	metrics.LogoutTotal.WithLabelValues("ok").Inc()
	return &authpb.LogoutResponse{Success: true}, nil
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
