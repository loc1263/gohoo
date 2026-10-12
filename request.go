package gohoo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// GetJSON hace un GET autenticado (cookie + crumb) a un endpoint de Yahoo
// Finance que gohoo no cubre y devuelve el cuerpo tal cual.
//
// path es relativo al host del cliente (p. ej. "/v7/finance/quote"; ver
// WithBaseURL) o una URL absoluta. q se agrega a la query (puede ser nil) y
// el crumb vigente se agrega como "crumb" en cada intento, reemplazando
// cualquier "crumb" de q. Usa la misma sesión y RetryPolicy que el resto
// de los métodos: reintenta errores de red, 429 y 5xx y renueva el crumb
// ante un HTTP 401. Una respuesta distinta de 200 se devuelve como
// *StatusError (clasificable con errors.Is).
func (c *Client) GetJSON(ctx context.Context, path string, q url.Values) ([]byte, error) {
	return c.authedRequest(ctx, http.MethodGet, path, q, nil, nil)
}

// PostJSON es GetJSON con método POST: body se serializa con
// encoding/json y se manda con Content-Type application/json. Si body es
// nil se manda sin cuerpo.
func (c *Client) PostJSON(ctx context.Context, path string, q url.Values, body any) ([]byte, error) {
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			return nil, fmt.Errorf("gohoo: cuerpo de %s: %w", path, err)
		}
	}
	return c.authedRequest(ctx, http.MethodPost, path, q, raw, nil)
}

// authedRequest es GetJSON/PostJSON con un check opcional del cuerpo (ver doAuthedCheck).
func (c *Client) authedRequest(ctx context.Context, method, path string, q url.Values, body []byte, check func([]byte) error) ([]byte, error) {
	endpoint, base, err := c.resolve(path, q)
	if err != nil {
		return nil, err
	}
	if err := c.ensureCrumb(ctx); err != nil {
		return nil, fmt.Errorf("gohoo: %s: %w", endpoint, err)
	}
	return c.doAuthedCheck(ctx, method, func() string {
		v := make(url.Values, len(base)+1)
		for k, vs := range base {
			v[k] = append([]string(nil), vs...)
		}
		v.Set("crumb", c.getCrumb())
		return endpoint + "?" + v.Encode()
	}, body, check)
}

// resolve arma la URL sin query de path y une la query que traiga path con q.
func (c *Client) resolve(path string, q url.Values) (endpoint string, query url.Values, err error) {
	full := path
	if !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		full = c.baseURL() + path
	}
	u, err := url.Parse(full)
	if err != nil || u.Host == "" {
		return "", nil, fmt.Errorf("gohoo: ruta inválida %q", path)
	}
	query = u.Query()
	for k, vs := range q {
		query[k] = append([]string(nil), vs...)
	}
	u.RawQuery, u.Fragment = "", ""
	return u.String(), query, nil
}
