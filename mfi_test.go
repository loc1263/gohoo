package gohoo

import (
	"math"
	"testing"
)

func TestMFIKnownValues(t *testing.T) {
	// H=L=C, así que el precio típico es el cierre.
	closes := []float64{10, 11, 10, 12, 12, 13}
	vols := []float64{100, 100, 200, 100, 50, 100}
	got := MFI(closes, closes, closes, vols, 3)
	if len(got) != len(closes) {
		t.Fatalf("len = %d", len(got))
	}
	for i := 0; i < 3; i++ {
		mustNaN(t, "MFI", i, got[i])
	}
	// i=3: barras 1..3 -> +1100 (11), -2000 (10), +1200 (12): pos=2300 neg=2000
	want3 := 100 - 100/(1+2300.0/2000.0)
	// i=4: barras 2..4 -> -2000, +1200, flat(12): pos=1200 neg=2000
	want4 := 100 - 100/(1+1200.0/2000.0)
	// i=5: barras 3..5 -> +1200, flat, +1300: pos=2500 neg=0 -> 100
	for i, want := range map[int]float64{3: want3, 4: want4, 5: 100} {
		if !almostEqual(got[i], want) {
			t.Errorf("MFI[%d] = %v, want %v", i, got[i], want)
		}
	}
}

func TestMFIOnlyDownIsZero(t *testing.T) {
	c := []float64{5, 4, 3, 2, 1}
	got := MFI(c, c, c, []float64{1, 1, 1, 1, 1}, 3)
	if !almostEqual(got[4], 0) {
		t.Errorf("MFI = %v, want 0", got[4])
	}
}

func TestMFIEdgeCases(t *testing.T) {
	c := []float64{1, 2, 3}
	if got := MFI(c, c, c, []float64{1, 1}, 2); got != nil {
		t.Errorf("columnas de distinto largo: %v, want nil", got)
	}
	for i, v := range MFI(c, c, c, c, 0) {
		mustNaN(t, "period 0", i, v)
	}
	for i, v := range MFI(c, c, c, c, 3) { // period >= len: todo NaN
		mustNaN(t, "period 3", i, v)
	}
	flat := []float64{5, 5, 5, 5}
	for i, v := range MFI(flat, flat, flat, flat, 2) { // sin flujo en ninguna dirección
		mustNaN(t, "flat", i, v)
	}
	nan := []float64{1, 2, math.NaN(), 4, 5, 6}
	got := MFI(nan, nan, nan, []float64{1, 1, 1, 1, 1, 1}, 2)
	for _, i := range []int{2, 3} { // ventanas que contienen el NaN
		mustNaN(t, "NaN", i, got[i])
	}
	if math.IsNaN(got[5]) {
		t.Errorf("MFI[5] = NaN, la ventana ya no contiene el NaN")
	}
}
