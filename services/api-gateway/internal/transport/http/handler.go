// api-gateway/internal/transport/http/handler.go
package http

import (
	"encoding/json"
	"net/http"
	"time"

	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"
	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"
	commonpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/common"

	intervalutil "github.com/YaganovValera/analytics-system/common/interval"

	"google.golang.org/protobuf/types/known/timestamppb"
)

type Handler struct {
	auth      authpb.AuthServiceClient
	analytics analyticspb.AnalyticsServiceClient
}

func NewHandler(auth authpb.AuthServiceClient, analytics analyticspb.AnalyticsServiceClient) *Handler {
	return &Handler{auth: auth, analytics: analytics}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	resp, err := h.auth.Login(r.Context(), &authpb.LoginRequest{
		Username: req.Username,
		Password: req.Password,
		Metadata: extractMetadata(r),
	})
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, loginResponse{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresIn:    resp.ExpiresIn,
	})
}

type registerRequest struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	Roles    []string `json:"roles"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	resp, err := h.auth.Register(r.Context(), &authpb.RegisterRequest{
		Username: req.Username,
		Password: req.Password,
		Roles:    req.Roles,
		Metadata: extractMetadata(r),
	})
	if err != nil {
		http.Error(w, "registration failed", http.StatusBadRequest)
		return
	}
	writeJSON(w, loginResponse{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresIn:    resp.ExpiresIn,
	})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	resp, err := h.auth.RefreshToken(r.Context(), &authpb.RefreshTokenRequest{
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		http.Error(w, "refresh failed", http.StatusUnauthorized)
		return
	}
	writeJSON(w, loginResponse{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresIn:    resp.ExpiresIn,
	})
}

func (h *Handler) GetCandles(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("symbol")
	interval := r.URL.Query().Get("interval")
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")

	if symbol == "" || interval == "" || start == "" || end == "" {
		http.Error(w, "missing query parameters", http.StatusBadRequest)
		return
	}

	startTs, err1 := time.Parse(time.RFC3339, start)
	endTs, err2 := time.Parse(time.RFC3339, end)
	if err1 != nil || err2 != nil {
		http.Error(w, "invalid time format", http.StatusBadRequest)
		return
	}

	protoInterval, err := intervalutil.ToProto(intervalutil.Interval(interval))
	if err != nil {
		http.Error(w, "invalid interval", http.StatusBadRequest)
		return
	}

	resp, err := h.analytics.GetCandles(r.Context(), &analyticspb.QueryCandlesRequest{
		Symbol:   symbol,
		Interval: protoInterval,
		Start:    timestamppb.New(startTs),
		End:      timestamppb.New(endTs),
		Pagination: &commonpb.Pagination{
			PageSize: 500,
		},
		Metadata: extractMetadata(r),
	})
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func extractMetadata(r *http.Request) *commonpb.RequestMetadata {
	return &commonpb.RequestMetadata{
		IpAddress: r.RemoteAddr,
		UserAgent: r.UserAgent(),
	}
}
