// api-gateway/internal/handler/auth_handlers.go
package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	authpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/auth"

	"github.com/YaganovValera/analytics-system/services/api-gateway/internal/response"
)

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
		response.BadRequest(w, "invalid json")
		return
	}

	resp, err := h.Auth.Login(r.Context(), &authpb.LoginRequest{
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		response.Unauthorized(w, "login failed")
		return
	}

	response.JSON(w, loginResponse{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresIn:    resp.ExpiresIn,
	})
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string   `json:"username"`
		Password string   `json:"password"`
		Roles    []string `json:"roles"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid json")
		return
	}

	resp, err := h.Auth.Register(r.Context(), &authpb.RegisterRequest{
		Username: req.Username,
		Password: req.Password,
		Roles:    req.Roles,
	})
	if err != nil {
		response.BadRequest(w, "registration failed")
		return
	}

	response.JSON(w, loginResponse{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresIn:    resp.ExpiresIn,
	})
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid json")
		return
	}

	resp, err := h.Auth.RefreshToken(r.Context(), &authpb.RefreshTokenRequest{
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		response.Unauthorized(w, "refresh failed")
		return
	}

	response.JSON(w, loginResponse{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresIn:    resp.ExpiresIn,
	})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	authResp, err := h.Auth.ValidateToken(ctx, &authpb.ValidateTokenRequest{
		Token: extractBearerToken(r),
	})
	if err != nil || !authResp.Valid {
		response.Unauthorized(w, "invalid token")
		return
	}

	response.JSON(w, struct {
		UserID    string   `json:"user_id"`
		Roles     []string `json:"roles"`
		ExpiresAt string   `json:"expires_at"`
		TraceID   string   `json:"trace_id,omitempty"`
	}{
		UserID:    authResp.Username,
		Roles:     authResp.Roles,
		ExpiresAt: authResp.ExpiresAt.AsTime().UTC().Format(time.RFC3339),
		TraceID:   authResp.Metadata.GetTraceId(),
	})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		response.BadRequest(w, "missing refresh_token")
		return
	}

	resp, err := h.Auth.Logout(r.Context(), &authpb.LogoutRequest{
		RefreshToken: req.RefreshToken,
	})
	if err != nil || !resp.Success {
		response.InternalError(w, "logout failed")
		return
	}

	w.WriteHeader(http.StatusNoContent) // 204
}

func extractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return ""
}
