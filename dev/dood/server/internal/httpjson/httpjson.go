// Package httpjson is the two-line "write a JSON response" / "write a JSON
// {"error": ...} response" pair every package's HTTP routes end with (main,
// jobs, deployment each had their own identical copy) - one implementation
// instead of three.
package httpjson

import (
	"encoding/json"
	"net/http"
)

// Write sends v as an uncached JSON response with the given status code.
func Write(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

// Err sends err as a JSON {"error": ...} body with the given status code.
func Err(w http.ResponseWriter, code int, err error) {
	Write(w, code, map[string]string{"error": err.Error()})
}
