// auth/internal/usecase/handler.go
package usecase

type Handler struct {
	Login    LoginHandler
	Refresh  RefreshTokenHandler
	Validate ValidateTokenHandler
	Revoke   RevokeTokenHandler
	Logout   LogoutHandler
	Register RegisterHandler
}

func NewHandler(
	login LoginHandler,
	refresh RefreshTokenHandler,
	validate ValidateTokenHandler,
	revoke RevokeTokenHandler,
	logout LogoutHandler,
	register RegisterHandler,
) Handler {
	return Handler{
		Login:    login,
		Refresh:  refresh,
		Validate: validate,
		Revoke:   revoke,
		Logout:   logout,
		Register: register,
	}
}
