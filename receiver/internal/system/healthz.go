package system

import (
	"log"
	"net/http"
)

// Healthz responds to an HTTP/S request with a 200 and content body "OK".
func Healthz(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("ok")); err != nil {
		log.Println(err)
	}
}
