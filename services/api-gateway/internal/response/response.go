// api-gateway/internal/response/response.go
package response

import (
	"encoding/json"
	"net/http"
)

type errorBody struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// JSON пишет успешный ответ.
func JSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	res := errorBody{}
	res.Error.Code = code
	res.Error.Message = msg
	_ = json.NewEncoder(w).Encode(res)
}

func BadRequest(w http.ResponseWriter, msg string)   { writeError(w, http.StatusBadRequest, msg) }
func Unauthorized(w http.ResponseWriter, msg string) { writeError(w, http.StatusUnauthorized, msg) }
func InternalError(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusInternalServerError, msg)
}
