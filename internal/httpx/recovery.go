package httpx

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// func Recovery(next http.Handler) http.Handler{
// 	return http.HandlerFunc(func(w http.ResponseWriter,r * http.Request){
// 		defer func() {
// 			if rec := recover(); rec!=nil {
// 				http.Error(w, http.StatusText(http.StatusInternalServerError),http.StatusInternalServerError)
// 			}
// 		} ()
// 		next.ServeHTTP(w,r)
// 	})
// }

func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				logger.Error("panic recovered",
					"request_id", RequestIDFromContext(r.Context()),
					"method", r.Method,
					"path", r.URL.Path,
					"panic", fmt.Sprintf("%v", rec),
					"stack", string(debug.Stack()),
				)
				http.Error(w, http.StatusText(http.StatusInternalServerError),
					http.StatusInternalServerError)
			}()
			next.ServeHTTP(w, r)
		})
	}
}
