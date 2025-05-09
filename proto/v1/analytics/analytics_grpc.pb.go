// proto/v1/analytics.proto

// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v3.21.12
// source: analytics/analytics.proto

package analyticspb

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	AnalyticsService_GetCandles_FullMethodName    = "/market.analytics.v1.AnalyticsService/GetCandles"
	AnalyticsService_StreamCandles_FullMethodName = "/market.analytics.v1.AnalyticsService/StreamCandles"
)

// AnalyticsServiceClient is the client API for AnalyticsService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
//
// AnalyticsService provides historical and streaming candle data.
type AnalyticsServiceClient interface {
	// Returns a page of historical candles matching the request.
	GetCandles(ctx context.Context, in *GetCandlesRequest, opts ...grpc.CallOption) (*GetCandlesResponse, error)
	// Streams candles (with errors) for the requested range.
	StreamCandles(ctx context.Context, in *GetCandlesRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[CandleEvent], error)
}

type analyticsServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewAnalyticsServiceClient(cc grpc.ClientConnInterface) AnalyticsServiceClient {
	return &analyticsServiceClient{cc}
}

func (c *analyticsServiceClient) GetCandles(ctx context.Context, in *GetCandlesRequest, opts ...grpc.CallOption) (*GetCandlesResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetCandlesResponse)
	err := c.cc.Invoke(ctx, AnalyticsService_GetCandles_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *analyticsServiceClient) StreamCandles(ctx context.Context, in *GetCandlesRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[CandleEvent], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &AnalyticsService_ServiceDesc.Streams[0], AnalyticsService_StreamCandles_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[GetCandlesRequest, CandleEvent]{ClientStream: stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type AnalyticsService_StreamCandlesClient = grpc.ServerStreamingClient[CandleEvent]

// AnalyticsServiceServer is the server API for AnalyticsService service.
// All implementations must embed UnimplementedAnalyticsServiceServer
// for forward compatibility.
//
// AnalyticsService provides historical and streaming candle data.
type AnalyticsServiceServer interface {
	// Returns a page of historical candles matching the request.
	GetCandles(context.Context, *GetCandlesRequest) (*GetCandlesResponse, error)
	// Streams candles (with errors) for the requested range.
	StreamCandles(*GetCandlesRequest, grpc.ServerStreamingServer[CandleEvent]) error
	mustEmbedUnimplementedAnalyticsServiceServer()
}

// UnimplementedAnalyticsServiceServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedAnalyticsServiceServer struct{}

func (UnimplementedAnalyticsServiceServer) GetCandles(context.Context, *GetCandlesRequest) (*GetCandlesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetCandles not implemented")
}
func (UnimplementedAnalyticsServiceServer) StreamCandles(*GetCandlesRequest, grpc.ServerStreamingServer[CandleEvent]) error {
	return status.Errorf(codes.Unimplemented, "method StreamCandles not implemented")
}
func (UnimplementedAnalyticsServiceServer) mustEmbedUnimplementedAnalyticsServiceServer() {}
func (UnimplementedAnalyticsServiceServer) testEmbeddedByValue()                          {}

// UnsafeAnalyticsServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to AnalyticsServiceServer will
// result in compilation errors.
type UnsafeAnalyticsServiceServer interface {
	mustEmbedUnimplementedAnalyticsServiceServer()
}

func RegisterAnalyticsServiceServer(s grpc.ServiceRegistrar, srv AnalyticsServiceServer) {
	// If the following call pancis, it indicates UnimplementedAnalyticsServiceServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&AnalyticsService_ServiceDesc, srv)
}

func _AnalyticsService_GetCandles_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetCandlesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AnalyticsServiceServer).GetCandles(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AnalyticsService_GetCandles_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AnalyticsServiceServer).GetCandles(ctx, req.(*GetCandlesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AnalyticsService_StreamCandles_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(GetCandlesRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(AnalyticsServiceServer).StreamCandles(m, &grpc.GenericServerStream[GetCandlesRequest, CandleEvent]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type AnalyticsService_StreamCandlesServer = grpc.ServerStreamingServer[CandleEvent]

// AnalyticsService_ServiceDesc is the grpc.ServiceDesc for AnalyticsService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var AnalyticsService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "market.analytics.v1.AnalyticsService",
	HandlerType: (*AnalyticsServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetCandles",
			Handler:    _AnalyticsService_GetCandles_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "StreamCandles",
			Handler:       _AnalyticsService_StreamCandles_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "analytics/analytics.proto",
}
