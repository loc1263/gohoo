package gohoo

import (
	"fmt"
	"strconv"
	"time"
)

// RawValue extrae el campo "raw" de los objetos {"raw":x,"fmt":"..."} que usa
// Yahoo Finance para casi todos los números, o devuelve el valor tal cual si
// ya es un escalar (string, bool, número plano).
func RawValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
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

// GetPath navega un árbol de map[string]interface{} siguiendo las claves
// dadas, devolviendo nil si alguna no existe o no es un mapa.
func GetPath(m map[string]interface{}, keys ...string) interface{} {
	var cur interface{} = m
	for _, k := range keys {
		cm, ok := cur.(map[string]interface{})
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
func Field(m map[string]interface{}, key string) interface{} {
	if m == nil {
		return nil
	}
	return RawValue(m[key])
}

func AsFloat(v interface{}) (float64, bool) {
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

func AsInt64(v interface{}) (int64, bool) {
	f, ok := AsFloat(v)
	if !ok {
		return 0, false
	}
	return int64(f), true
}

func AsString(v interface{}) string {
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
func EpochToDate(v interface{}) string {
	f, ok := AsFloat(v)
	if !ok {
		return ""
	}
	return time.Unix(int64(f), 0).UTC().Format("2006-01-02")
}

// EpochToDateTimeNY convierte un timestamp Unix a "YYYY-MM-DD HH:MM:SS-07:00"
// en la zona America/New_York (zona horaria de las fechas de resultados).
func EpochToDateTimeNY(v interface{}, loc *time.Location) string {
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
func EpochToMidnight(v interface{}) string {
	d := EpochToDate(v)
	if d == "" {
		return ""
	}
	return d + " 00:00:00"
}

func DateStringToMidnight(s string) string {
	if s == "" {
		return ""
	}
	return s + " 00:00:00"
}

func ParseDateOnly(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}
