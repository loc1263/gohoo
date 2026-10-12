package gohoo

import "context"

// InfoModules son los módulos de quoteSummary que pide Info por defecto, en
// orden de prioridad para FlattenInfo: si una clave aparece en varios, gana
// el primero. summaryDetail va primero porque algunos campos de
// defaultKeyStatistics vienen corruptos en ciertas acciones (p. ej. un
// forwardPE de 11836 en vez de 11,2 en acciones chilenas).
//
// Es una función (no una variable) para que nadie altere el valor por
// defecto de otro paquete; cada llamada devuelve una copia nueva.
func InfoModules() []string {
	return []string{"summaryDetail", "price", "defaultKeyStatistics", "financialData", "assetProfile", "quoteType"}
}

// InfoOption configura Info y FlattenInfo.
type InfoOption func(*infoConfig)

type infoConfig struct {
	modules      []string
	percentYield bool
	trailingPEG  bool
}

// InfoWithModules cambia los módulos que se piden y aplanan, y su orden de
// prioridad (el primero gana). Por defecto, InfoModules().
func InfoWithModules(modules ...string) InfoOption {
	return func(c *infoConfig) { c.modules = append([]string(nil), modules...) }
}

// InfoPercentYield entrega dividendYield en porcentaje (2,79 en vez de
// 0,0279), como Ticker.info de yfinance; si falta (los ETF no lo traen en
// summaryDetail), se toma "yield". Por defecto los valores quedan tal como
// los manda Yahoo (fracción).
func InfoPercentYield() InfoOption {
	return func(c *infoConfig) { c.percentYield = true }
}

// InfoWithTrailingPEG hace que Info agregue "trailingPegRatio" (que
// quoteSummary no trae) con una request más a fundamentals-timeseries, como
// Ticker.info de yfinance. Si esa request falla o Yahoo no tiene el dato, la
// clave no se agrega y Info no falla. FlattenInfo ignora esta opción.
func InfoWithTrailingPEG() InfoOption {
	return func(c *infoConfig) { c.trailingPEG = true }
}

func newInfoConfig(opts []InfoOption) infoConfig {
	var cfg infoConfig
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	if cfg.modules == nil {
		cfg.modules = InfoModules()
	}
	return cfg
}

// FlattenInfo aplana una respuesta de QuoteSummary en un mapa clave → valor
// al estilo de Ticker.info de yfinance: recorre los módulos en orden de
// prioridad (ver InfoModules e InfoWithModules), la primera aparición de
// cada clave gana, los números quedan sin el envoltorio {raw, fmt} y los
// valores nulos u objetos sin "raw" se omiten. Los módulos de qs que no
// están en la lista se ignoran. No modifica qs.
func FlattenInfo(qs map[string]any, opts ...InfoOption) map[string]any {
	cfg := newInfoConfig(opts)
	out := map[string]any{}
	for _, mod := range cfg.modules {
		m, _ := qs[mod].(map[string]any)
		for k, v := range m {
			if _, taken := out[k]; taken {
				continue
			}
			if rv := RawValue(v); rv != nil {
				out[k] = rv
			}
		}
	}
	if cfg.percentYield {
		if f, ok := AsFloat(out["dividendYield"]); ok {
			out["dividendYield"] = f * 100
		} else if f, ok := AsFloat(out["yield"]); ok {
			out["dividendYield"] = f * 100
		}
	}
	return out
}

// Info pide quoteSummary con los módulos de InfoModules (o los de
// InfoWithModules) y lo aplana con FlattenInfo: un mapa plano de datos del
// ticker como Ticker.info de yfinance.
//
//	info, err := client.Info(ctx, "AAPL", gohoo.InfoPercentYield(), gohoo.InfoWithTrailingPEG())
//	pe, _ := gohoo.AsFloat(info["trailingPE"])
func (c *Client) Info(ctx context.Context, ticker string, opts ...InfoOption) (map[string]any, error) {
	cfg := newInfoConfig(opts)
	qs, err := c.QuoteSummaryContext(ctx, ticker, cfg.modules)
	if err != nil {
		return nil, err
	}
	info := FlattenInfo(qs, opts...)
	if cfg.trailingPEG {
		if v, ok, perr := c.TrailingPEG(ctx, ticker); perr == nil && ok {
			info["trailingPegRatio"] = v
		}
	}
	return info, nil
}
