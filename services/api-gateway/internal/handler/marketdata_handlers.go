// api-gateway/internal/handler/marketdata_handlers.go
package handler

import (
	"net/http"
	"strconv"
	"time"

	commonpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/common"
	mdpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/marketdata"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/YaganovValera/analytics-system/services/api-gateway/internal/response"
	"github.com/YaganovValera/analytics-system/services/api-gateway/internal/transport"
)

func (h *Handler) GetOrderBook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		response.BadRequest(w, "missing ?symbol=")
		return
	}

	// Период запроса
	start, err := parseTimeQuery(r.URL.Query().Get("start"), time.Now().Add(-5*time.Minute))
	if err != nil {
		response.BadRequest(w, "invalid start timestamp")
		return
	}
	end, err := parseTimeQuery(r.URL.Query().Get("end"), time.Now())
	if err != nil {
		response.BadRequest(w, "invalid end timestamp")
		return
	}
	if !start.Before(end) {
		response.BadRequest(w, "start must be before end")
		return
	}

	// Пагинация
	pageSize := parseInt32Query(r, "page_size", 100)
	pageToken := r.URL.Query().Get("page_token")

	// gRPC запрос
	resp, err := h.MarketData.GetOrderBook(ctx, &mdpb.GetOrderBookRequest{
		Symbol: symbol,
		Start:  timestamppb.New(start),
		End:    timestamppb.New(end),
		Pagination: &commonpb.Pagination{
			PageSize:  pageSize,
			PageToken: pageToken,
		},
		Metadata: transport.ExtractMetadataFromContext(ctx),
	})
	if err != nil {
		response.InternalError(w, "failed to fetch order book")
		return
	}

	response.JSON(w, resp)
}

func parseTimeQuery(s string, def time.Time) (time.Time, error) {
	if s == "" {
		return def, nil
	}
	return time.Parse(time.RFC3339, s)
}

func parseInt32Query(r *http.Request, key string, def int32) int32 {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return int32(n)
		}
	}
	return def
}
