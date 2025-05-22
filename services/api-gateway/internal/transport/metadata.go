package transport

import (
	"context"

	"github.com/YaganovValera/analytics-system/common/ctxkeys"
	commonpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/common"
)

func ExtractMetadataFromContext(ctx context.Context) *commonpb.RequestMetadata {
	return &commonpb.RequestMetadata{
		TraceId:   getCtx(ctx, ctxkeys.TraceIDKey),
		IpAddress: getCtx(ctx, ctxkeys.IPAddressKey),
		UserAgent: getCtx(ctx, ctxkeys.UserAgentKey),
	}
}

func getCtx(ctx context.Context, key any) string {
	val, _ := ctx.Value(key).(string)
	return val
}
