// api-gateway/internal/handler/analytics_handlers.go
package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/YaganovValera/analytics-system/common/interval"

	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"
	commonpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/common"

	"github.com/YaganovValera/analytics-system/services/api-gateway/internal/response"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func (h *Handler) GetCandles(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("symbol")
	intvl := r.URL.Query().Get("interval")
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")

	if symbol == "" || intvl == "" || start == "" || end == "" {
		response.BadRequest(w, "missing query parameters")
		return
	}

	startTs, err1 := time.Parse(time.RFC3339, start)
	endTs, err2 := time.Parse(time.RFC3339, end)
	if err1 != nil || err2 != nil {
		response.BadRequest(w, "invalid time format")
		return
	}

	protoIntvl, err := interval.ToProto(interval.Interval(intvl))
	if err != nil {
		response.BadRequest(w, "invalid interval")
		return
	}

	resp, err := h.Analytics.GetCandles(r.Context(), &analyticspb.QueryCandlesRequest{
		Symbol:   symbol,
		Interval: protoIntvl,
		Start:    timestamppb.New(startTs),
		End:      timestamppb.New(endTs),
		Pagination: &commonpb.Pagination{
			PageSize: 500,
		},
	})
	if err != nil {
		response.InternalError(w, "query failed")
		return
	}

	response.JSON(w, resp)
}

func (h *Handler) GetSymbols(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	pageSize := int32(100)
	if v := r.URL.Query().Get("page_size"); v != "" {
		if ps, err := strconv.Atoi(v); err == nil && ps > 0 {
			pageSize = int32(ps)
		}
	}
	pageToken := r.URL.Query().Get("page_token")

	resp, err := h.Common.ListSymbols(ctx, &commonpb.ListSymbolsRequest{
		Pagination: &commonpb.Pagination{
			PageSize:  pageSize,
			PageToken: pageToken,
		},
	})
	if err != nil {
		response.InternalError(w, "failed to fetch symbols")
		return
	}

	response.JSON(w, struct {
		Symbols       []string `json:"symbols"`
		NextPageToken string   `json:"next_page_token,omitempty"`
	}{
		Symbols:       resp.Symbols,
		NextPageToken: resp.NextPageToken,
	})
}
