package gohoo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func TestGetJSONAddsCrumbAndQuery(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v7/finance/quote" {
			t.Errorf("request inesperada: %s %s", r.Method, r.URL.Path)
		}
		got = r.URL.Query()
		io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	body, err := c.GetJSON(context.Background(), "/v7/finance/quote?lang=en-US", url.Values{"symbols": {"AAPL,MSFT"}, "crumb": {"viejo"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("cuerpo = %s", body)
	}
	if got.Get("crumb") != "test-crumb" || got.Get("symbols") != "AAPL,MSFT" || got.Get("lang") != "en-US" {
		t.Errorf("query = %v", got)
	}
}

func TestGetJSONRelativePathWithoutSlashAndAbsoluteURL(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	if _, err := c.GetJSON(context.Background(), "v1/x", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetJSON(context.Background(), "https://query1.finance.yahoo.com/v1/y", nil); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "/v1/x" || paths[1] != "/v1/y" {
		t.Errorf("paths = %v", paths)
	}
}

func TestGetJSONInvalidPathBeforeNetwork(t *testing.T) {
	c := &Client{crumb: "x"}
	if _, err := c.GetJSON(context.Background(), "https://", nil); err == nil {
		t.Fatal("se esperaba error por ruta inválida")
	}
}

func TestPostJSONSendsBodyAndRetries5xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("request inesperada: %s %q", r.Method, r.Header.Get("Content-Type"))
		}
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil || got["size"] != float64(5) {
			t.Errorf("cuerpo = %v, %v", got, err)
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		io.WriteString(w, `{"ok":1}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	recordSleep(c)

	body, err := c.PostJSON(context.Background(), "/v1/finance/screener", nil, map[string]any{"size": 5})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"ok":1}` || calls.Load() != 2 {
		t.Errorf("cuerpo = %s, llamadas = %d (el reintento debe reenviar el cuerpo)", body, calls.Load())
	}
}

func TestPostJSONRenewsCrumbOn401(t *testing.T) {
	var crumbs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/test/getcrumb":
			io.WriteString(w, "NUEVO")
		case "/v1/finance/screener":
			crumbs = append(crumbs, r.URL.Query().Get("crumb"))
			if r.URL.Query().Get("crumb") != "NUEVO" {
				w.WriteHeader(http.StatusUnauthorized)
				io.WriteString(w, `{"finance":{"result":null,"error":{"code":"Unauthorized","description":"Invalid Crumb"}}}`)
				return
			}
			io.WriteString(w, `{}`)
		default:
			w.WriteHeader(http.StatusNotFound) // fc.yahoo.com
		}
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	recordSleep(c)

	if _, err := c.PostJSON(context.Background(), "/v1/finance/screener", nil, struct{}{}); err != nil {
		t.Fatal(err)
	}
	if len(crumbs) != 2 || crumbs[0] != "test-crumb" || crumbs[1] != "NUEVO" {
		t.Errorf("crumbs = %v", crumbs)
	}
}

func TestPostJSONLazyBootstrapAndTypedErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/test/getcrumb":
			io.WriteString(w, "C1")
		case "/limite":
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	p := DefaultRetryPolicy()
	p.RetryRateLimit = false
	c := newRewritingClient(t, srv, WithLazyBootstrap(), WithRetryPolicy(p))

	_, err := c.PostJSON(context.Background(), "/limite", nil, nil)
	if !IsRateLimit(err) {
		t.Errorf("err = %v, se esperaba ErrRateLimit", err)
	}
	if c.getCrumb() != "C1" {
		t.Errorf("el bootstrap perezoso no corrió (crumb %q)", c.getCrumb())
	}
	_, err = c.GetJSON(context.Background(), "/nada", nil)
	var se *StatusError
	if !errors.Is(err, ErrNotFound) || !errors.As(err, &se) || se.StatusCode != 404 {
		t.Errorf("err = %v, se esperaba *StatusError 404", err)
	}
}

func TestPostJSONMarshalError(t *testing.T) {
	c := &Client{crumb: "x"}
	if _, err := c.PostJSON(context.Background(), "/x", nil, make(chan int)); err == nil {
		t.Fatal("se esperaba error al serializar")
	}
}
