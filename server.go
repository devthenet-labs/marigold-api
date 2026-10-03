// Copyright 2026 DevTheNet Labs.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

// greetingMessage is what GET /api/greeting says.
const greetingMessage = "Hello from Marigold"

// nameInput matches a visitor's name: 1-40 Unicode letters, spaces,
// hyphens and apostrophes. Anything else is a 400.
var nameInput = regexp.MustCompile(`^[\p{L} '-]{1,40}$`)

// greeting is the body of GET /api/greeting.
type greeting struct {
	Message  string `json:"message"`
	ServedAt string `json:"servedAt"`
	Revision string `json:"revision"`
}

// newHandler serves the API. Every API route lives under /api: in a preview
// the load balancer routes the /api prefix to this service without rewriting
// the path, and the web app calls it from the browser on the same origin.
// Anything else is a JSON 404.
func newHandler(revision string, now func() time.Time) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/greeting", readOnly(func(w http.ResponseWriter, r *http.Request) {
		message := greetingMessage
		if r.URL.Query().Has("name") {
			name := r.URL.Query().Get("name")
			if !nameInput.MatchString(name) {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error": "name must be 1-40 letters, spaces, hyphens or apostrophes",
				})
				return
			}
			message = fmt.Sprintf("Hello, %s!", name)
		}
		writeJSON(w, http.StatusOK, greeting{
			Message:  message,
			ServedAt: now().UTC().Format(time.RFC3339),
			Revision: revision,
		})
	}))
	mux.HandleFunc("/healthz", readOnly(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}

// readOnly admits GET and HEAD and answers anything else with a JSON 405.
func readOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
