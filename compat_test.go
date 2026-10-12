package gohoo

import "time"

// Este archivo fija las firmas de la API pública publicada en v0.1.0: si
// alguna cambia, el paquete deja de compilar en los tests. No se edita
// para "acomodar" un cambio; un fallo acá es un cambio incompatible.
var (
	_ func() (*Client, error) = NewClient

	_ func(*Client, string, []string) (map[string]any, error)                                        = (*Client).QuoteSummary
	_ func(*Client, string, []string) (map[string]any, error)                                        = (*Client).FundamentalsTimeseries
	_ func(*Client, string, string, string) (map[string]any, error)                                  = (*Client).Chart
	_ func(*Client, string, string, string) ([]OHLCV, map[string]float64, map[string]float64, error) = (*Client).ChartSeries

	_ func(map[string]any) (map[string]any, bool) = FirstResult
	_ func([]OHLCV, float64) []Gap                = DetectGaps
	_ func([]OHLCV) []IndicatorSet                = ComputeIndicators

	_ func(any) any                       = RawValue
	_ func(map[string]any, ...string) any = GetPath
	_ func(map[string]any, string) any    = Field
	_ func(any) (float64, bool)           = AsFloat
	_ func(any) (int64, bool)             = AsInt64
	_ func(any) string                    = AsString
	_ func(any) string                    = EpochToDate
	_ func(any, *time.Location) string    = EpochToDateTimeNY
	_ func(any) string                    = EpochToMidnight
	_ func(string) string                 = DateStringToMidnight
	_ func(string) (time.Time, error)     = ParseDateOnly
)

// Campos exportados de los tipos públicos de v0.1.0 (y del error interno
// que los tests inspeccionan con errors.As).
var (
	_ = OHLCV{Time: time.Time{}, Open: nil, High: nil, Low: nil, Close: nil, AdjClose: nil, Volume: nil}
	_ = Gap{From: time.Time{}, To: time.Time{}, Esperado: 0, Real: 0}
	_ = httpStatusError{StatusCode: 0, URL: "", Body: ""}
)
