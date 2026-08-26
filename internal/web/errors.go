package web

import "net/http"

func methodNotAllowed(w http.ResponseWriter) {
	w.Header().Set("Allow", "GET, POST")
	w.WriteHeader(http.StatusMethodNotAllowed)
}
