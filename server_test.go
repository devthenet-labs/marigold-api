// Copyright 2026 DevTheNet Labs.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
	"time"
)

// fixedClock returns a non-UTC instant, so the tests also prove servedAt is
// rendered in UTC.
func fixedClock() time.Time {
	return time.Date(2026, 10, 3, 14, 30, 5, 0, time.FixedZone("UTC+2", 2*60*60))
}

func serve(h http.Handler, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func TestGreeting(t *testing.T) {
	w := serve(newHandler("0123456789abcdef", fixedClock), http.MethodGet, "/api/greeting")
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status=%d type=%q", w.Code, w.Header().Get("Content-Type"))
	}
	// Exactly these three keys: the web app and the preview demo rely on them.
	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"message": greetingMessage, "servedAt": "2026-10-03T12:30:05Z", "revision": "0123456789abcdef"}
	if !reflect.DeepEqual(raw, want) {
		t.Fatalf("body=%v, want %v", raw, want)
	}
}

func TestRoutes(t *testing.T) {
	h := newHandler("test", fixedClock)
	for _, tc := range []struct {
		method, path, contains string
		status                 int
	}{
		{http.MethodGet, "/healthz", `{"status":"ok"}`, http.StatusOK},
		// Only /api/greeting and /healthz exist; the API serves nothing at
		// the root (that path belongs to the web app in a preview).
		{http.MethodGet, "/", `{"error":"not found"}`, http.StatusNotFound},
		{http.MethodGet, "/api", `{"error":"not found"}`, http.StatusNotFound},
		{http.MethodGet, "/api/", `{"error":"not found"}`, http.StatusNotFound},
		{http.MethodGet, "/greeting", `{"error":"not found"}`, http.StatusNotFound},
		{http.MethodGet, "/api/greeting/extra", `{"error":"not found"}`, http.StatusNotFound},
		{http.MethodPost, "/api/greeting", `{"error":"method not allowed"}`, http.StatusMethodNotAllowed},
		{http.MethodDelete, "/healthz", `{"error":"method not allowed"}`, http.StatusMethodNotAllowed},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			w := serve(h, tc.method, tc.path)
			if w.Code != tc.status || w.Header().Get("Content-Type") != "application/json" || !strings.Contains(w.Body.String(), tc.contains) {
				t.Fatalf("status=%d type=%q body=%q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
			}
			if tc.status == http.StatusMethodNotAllowed && w.Header().Get("Allow") != "GET, HEAD" {
				t.Fatalf("Allow=%q", w.Header().Get("Allow"))
			}
			assertHeaders(t, w)
		})
	}
}

func assertHeaders(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "no-store" ||
		w.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("missing security headers")
	}
	if w.Header().Get("Set-Cookie") != "" {
		t.Fatal("the API must not set cookies")
	}
	// Same-origin only: the browser reaches the API through the web app's host.
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("the API must not opt into cross-origin reads")
	}
}

func TestRevisionIsJSONEscaped(t *testing.T) {
	w := serve(newHandler(`<script>alert("no")</script>`, fixedClock), http.MethodGet, "/api/greeting")
	if strings.Contains(w.Body.String(), "<script>") {
		t.Fatalf("body=%q", w.Body.String())
	}
	var got greeting
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.Revision != `<script>alert("no")</script>` {
		t.Fatalf("greeting=%+v error=%v", got, err)
	}
}

func TestHEAD(t *testing.T) {
	server := httptest.NewServer(newHandler("test", fixedClock))
	t.Cleanup(server.Close)
	for _, path := range []string{"/api/greeting", "/healthz"} {
		response, err := server.Client().Head(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK || len(body) != 0 {
			t.Fatalf("HEAD %s: status=%d bytes=%d error=%v", path, response.StatusCode, len(body), readErr)
		}
	}
}

// unknownPath is a clean URL path that is neither route.
type unknownPath string

func (unknownPath) Generate(r *rand.Rand, size int) reflect.Value {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789-_."
	for {
		var b strings.Builder
		for range 1 + r.Intn(4) {
			b.WriteByte('/')
			for range 1 + r.Intn(1+size%12) {
				b.WriteByte(alphabet[r.Intn(len(alphabet))])
			}
		}
		if r.Intn(4) == 0 {
			b.WriteByte('/')
		}
		p := b.String()
		// "." and ".." segments are cleaned and redirected by the mux.
		if p != "/api/greeting" && p != "/healthz" && !strings.Contains(p+"/", "/./") && !strings.Contains(p+"/", "/../") {
			return reflect.ValueOf(unknownPath(p))
		}
	}
}

// Property: every path other than the two routes is a JSON 404 that decodes
// to {"error":"not found"}. Seeded, so the gate is deterministic.
func TestUnknownPathsAreJSON404s(t *testing.T) {
	h := newHandler("test", fixedClock)
	property := func(p unknownPath) bool {
		w := serve(h, http.MethodGet, string(p))
		var body map[string]string
		dec := json.NewDecoder(bytes.NewReader(w.Body.Bytes()))
		return w.Code == http.StatusNotFound && w.Header().Get("Content-Type") == "application/json" &&
			dec.Decode(&body) == nil && reflect.DeepEqual(body, map[string]string{"error": "not found"})
	}
	if err := quick.Check(property, &quick.Config{MaxCount: 500, Rand: rand.New(rand.NewSource(20261003))}); err != nil {
		t.Fatal(err)
	}
}
