// auth/internal/usecase/handler.go
package usecase

type Handler struct {
	Login    LoginHandler
	Refresh  RefreshTokenHandler
	Validate ValidateTokenHandler
	Revoke   RevokeTokenHandler
	Logout   LogoutHandler
}

func NewHandler(
	login LoginHandler,
	refresh RefreshTokenHandler,
	validate ValidateTokenHandler,
	revoke RevokeTokenHandler,
	logout LogoutHandler,
) Handler {
	return Handler{
		Login:    login,
		Refresh:  refresh,
		Validate: validate,
		Revoke:   revoke,
		Logout:   logout,
	}
}
