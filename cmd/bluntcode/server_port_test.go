package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func portOfServer(t *testing.T, server *httptest.Server) int {
	t.Helper()
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(endpoint.Port())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func TestHandoffToPortFindsLiveServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/meta" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"version":"test"}`)
	}))
	defer server.Close()
	if !handoffToPort(portOfServer(t, server), true) {
		t.Fatal("handoff must succeed against a live server")
	}
}

func TestHandoffToPortRejectsForeignAndDeadPorts(t *testing.T) {
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	}))
	defer foreign.Close()
	if handoffToPort(portOfServer(t, foreign), true) {
		t.Fatal("handoff must not claim a non-Blunt-Code server")
	}
	held, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := held.Addr().(*net.TCPAddr).Port
	_ = held.Close()
	if handoffToPort(dead, true) {
		t.Fatal("handoff must not claim a dead port")
	}
	if handoffToPort(0, true) || handoffToPort(-1, true) {
		t.Fatal("handoff must reject non-positive ports")
	}
}
