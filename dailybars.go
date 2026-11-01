package gohoo

import (
	"context"
	"fmt"
	"time"
)

// DailyBars descarga las barras diarias de ticker entre start (inclusive) y
// end (exclusivo), ajustadas por splits y dividendos (AutoAdjusted) y con la
// hora llevada a la medianoche del día en la zona del exchange (AtMidnight).
// Es ChartRange con Interval "1d" más esos dos pasos.
//
// Si Yahoo responde sin ninguna barra, el error cumple
// errors.Is(err, ErrNotFound); los demás errores son los de ChartRange.
func (c *Client) DailyBars(ctx context.Context, ticker string, start, end time.Time) ([]Bar, error) {
	res, err := c.ChartRange(ctx, ticker, ChartParams{Start: start, End: end, Interval: "1d"})
	if err != nil {
		return nil, err
	}
	bars := AtMidnight(res.AutoAdjusted())
	if len(bars) == 0 {
		return nil, fmt.Errorf("gohoo: chart de %s sin barras diarias entre %s y %s: %w",
			ticker, start.Format(time.DateOnly), end.Format(time.DateOnly), ErrNotFound)
	}
	return bars, nil
}
