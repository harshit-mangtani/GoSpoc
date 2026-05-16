package httpx

import (
	"log/slog"
	"net/http"
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
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
			defer func(){
				if rec:=recover(); rec!=nil{
					logger.Error("panic recovered")
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			} ()
			next.ServeHTTP(w,r)
		})
	} 
}