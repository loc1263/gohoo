package gohoo

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// rewriteTransport reescribe el host/scheme de cada request hacia el
// httptest.Server de prueba, dejando intacto el resto de la URL (path y
// query). Así se pueden ejercitar los métodos del Client, que construyen
// URLs fijas apuntando a query2.finance.yahoo.com, sin tocar producción.
type rewriteTransport struct {
	base *url.URL
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = t.base.Scheme
	req.URL.Host = t.base.Host
	return http.DefaultTransport.RoundTrip(req)
}

// newTestClient crea un *Client cuyo http.Client redirige cualquier request
// al httptest.Server dado, sin pasar por el bootstrap real (cookies+crumb).
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}
	return &Client{
		http:  &http.Client{Transport: &rewriteTransport{base: base}},
		crumb: "test-crumb",
	}
}
