package gohoo

import "testing"

func TestPointHelpers(t *testing.T) {
	pts := []Point{
		{Date: "2024-12-31", PeriodType: "3M", Value: 4},
		{Date: "2024-09-30", PeriodType: "3M", Value: 3},
		{Date: "2024-06-30", PeriodType: "3M", Value: 2},
		{Date: "2024-03-31", PeriodType: "12M", Value: 100},
		{Date: "2023-12-31", PeriodType: "3M", Value: 1},
	}
	if p, ok := LatestPoint(pts); !ok || p.Date != "2024-12-31" {
		t.Errorf("LatestPoint = %+v, %v", p, ok)
	}
	if _, ok := LatestPoint(nil); ok {
		t.Error("LatestPoint(nil) ok = true")
	}
	if p, ok := PointOn(pts, "2024-06-30"); !ok || p.Value != 2 {
		t.Errorf("PointOn = %+v, %v", p, ok)
	}
	if _, ok := PointOn(pts, "1999-01-01"); ok {
		t.Error("PointOn(fecha ausente) ok = true")
	}
	q := FilterPeriodType(pts, "3M")
	if len(q) != 4 || q[0].Value != 4 || q[3].Value != 1 {
		t.Errorf("FilterPeriodType = %+v", q)
	}
	if len(pts) != 5 {
		t.Error("FilterPeriodType modificó la entrada")
	}
	if FilterPeriodType(pts, "TTM") != nil {
		t.Error("FilterPeriodType sin coincidencias debe ser nil")
	}
	if s, ok := SumLast(q, 4); !ok || s != 10 {
		t.Errorf("SumLast(4) = %v, %v", s, ok)
	}
	if _, ok := SumLast(q, 5); ok {
		t.Error("SumLast con menos puntos que n ok = true")
	}
	if _, ok := SumLast(q, 0); ok {
		t.Error("SumLast(0) ok = true")
	}
}
