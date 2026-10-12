package gohoo

import (
	"fmt"
	"strconv"
	"time"
)

// RawValue extrae el campo "raw" de los objetos {"raw":x,"fmt":"..."} que usa
// Yahoo Finance para casi todos los números, o devuelve el valor tal cual si
// ya es un escalar (string, bool, número plano).
func RawValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		if raw, ok := t["raw"]; ok {
			return raw
		}
		return nil
	case nil:
		return nil
	default:
		return t
	}
}

// GetPath navega un árbol de map[string]any siguiendo las claves
// dadas, devolviendo nil si alguna no existe o no es un mapa.
func GetPath(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		cm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = cm[k]
		if !ok {
			return nil
		}
	}
	return cur
}

// Field devuelve el valor "raw" de un módulo plano, p.ej Field(summaryDetail, "trailingPE").
func Field(m map[string]any, key string) any {
	if m == nil {
		return nil
	}
	return RawValue(m[key])
}

// AsFloat convierte v a float64 si es un float64, int, int64 o un string
// numérico; ok es false en cualquier otro caso (incluido nil). Para los
// objetos {raw, fmt} de Yahoo, aplicar antes RawValue.
func AsFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

// AsInt64 es AsFloat truncado a int64 (útil para timestamps Unix).
func AsInt64(v any) (int64, bool) {
	f, ok := AsFloat(v)
	if !ok {
		return 0, false
	}
	return int64(f), true
}

// AsString convierte v a texto: los strings tal cual, los números sin
// notación exponencial, los booleanos como "true"/"false", nil como "" y
// cualquier otro valor con fmt.
func AsString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// EpochToDate convierte un timestamp Unix (segundos) a "YYYY-MM-DD" en UTC.
func EpochToDate(v any) string {
	f, ok := AsFloat(v)
	if !ok {
		return ""
	}
	return time.Unix(int64(f), 0).UTC().Format("2006-01-02")
}

// EpochToDateTimeNY convierte un timestamp Unix a "YYYY-MM-DD HH:MM:SS-07:00"
// en la zona America/New_York (zona horaria de las fechas de resultados).
func EpochToDateTimeNY(v any, loc *time.Location) string {
	f, ok := AsFloat(v)
	if !ok {
		return ""
	}
	t := time.Unix(int64(f), 0)
	if loc != nil {
		t = t.In(loc)
	} else {
		t = t.UTC()
	}
	return t.Format("2006-01-02 15:04:05-07:00")
}

// EpochToMidnight da formato "YYYY-MM-DD HH:MM:SS" (siempre 00:00:00) para
// columnas de período de estados financieros.
func EpochToMidnight(v any) string {
	d := EpochToDate(v)
	if d == "" {
		return ""
	}
	return d + " 00:00:00"
}

// DateStringToMidnight agrega " 00:00:00" a una fecha "YYYY-MM-DD" (vacío si
// s es vacío), con el mismo formato que EpochToMidnight.
func DateStringToMidnight(s string) string {
	if s == "" {
		return ""
	}
	return s + " 00:00:00"
}

// ParseDateOnly interpreta una fecha "YYYY-MM-DD" (en UTC).
func ParseDateOnly(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}
