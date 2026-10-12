package gohoo

import "math"

// Returns devuelve los retornos simples v[i]/v[i-1] - 1, alineados con
// values: el primero es NaN, y también los que tienen un valor NaN o un
// valor anterior 0 o NaN.
func Returns(values []float64) []float64 {
	return periodReturns(values, func(cur, prev float64) float64 { return cur/prev - 1 })
}

// LogReturns devuelve los retornos logarítmicos ln(v[i]/v[i-1]), con las
// mismas reglas de NaN que Returns (también NaN si el cociente no es
// positivo).
func LogReturns(values []float64) []float64 {
	return periodReturns(values, func(cur, prev float64) float64 {
		if cur/prev <= 0 {
			return math.NaN()
		}
		return math.Log(cur / prev)
	})
}

func periodReturns(values []float64, f func(cur, prev float64) float64) []float64 {
	out := nanSlice(len(values))
	for i := 1; i < len(values); i++ {
		cur, prev := values[i], values[i-1]
		if math.IsNaN(cur) || math.IsNaN(prev) || prev == 0 {
			continue
		}
		out[i] = f(cur, prev)
	}
	return out
}

// Volatility es la desviación estándar muestral (n-1) de los retornos no
// NaN, anualizada con sqrt(periodsPerYear) (252 para retornos diarios, 52
// semanales, 12 mensuales; 1 para no anualizar). NaN si hay menos de dos
// retornos válidos o periodsPerYear <= 0.
func Volatility(returns []float64, periodsPerYear float64) float64 {
	if periodsPerYear <= 0 {
		return math.NaN()
	}
	var n int
	var mean, m2 float64
	for _, r := range returns { // algoritmo de Welford
		if math.IsNaN(r) {
			continue
		}
		n++
		d := r - mean
		mean += d / float64(n)
		m2 += d * (r - mean)
	}
	if n < 2 {
		return math.NaN()
	}
	return math.Sqrt(m2/float64(n-1)) * math.Sqrt(periodsPerYear)
}

// MaxDrawdown devuelve la mayor caída desde un máximo previo de la serie
// (precios o valor de un portafolio), como fracción <= 0 (-0,25 = -25 %),
// y los índices del máximo y del mínimo de esa caída. Ignora los NaN. Sin
// caídas devuelve (0, -1, -1); sin valores válidos, (NaN, -1, -1).
func MaxDrawdown(values []float64) (dd float64, peak, trough int) {
	peak, trough = -1, -1
	runPeak := -1
	valid := false
	for i, v := range values {
		if math.IsNaN(v) {
			continue
		}
		valid = true
		if runPeak < 0 || v > values[runPeak] {
			runPeak = i
			continue
		}
		if values[runPeak] <= 0 {
			continue
		}
		if d := v/values[runPeak] - 1; d < dd {
			dd, peak, trough = d, runPeak, i
		}
	}
	if !valid {
		return math.NaN(), -1, -1
	}
	return dd, peak, trough
}
