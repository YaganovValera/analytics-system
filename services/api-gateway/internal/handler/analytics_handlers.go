// api-gateway/internal/handler/analytics_handlers.go
package handler

import (
	"net/http"
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
