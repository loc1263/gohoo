package yahoo

import (
	"testing"
	"time"
)

func TestRawValue(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want interface{}
	}{
		{"wrapped raw", map[string]interface{}{"raw": 12.5, "fmt": "12.50"}, 12.5},
		{"map without raw", map[string]interface{}{"fmt": "n/a"}, nil},
		{"plain scalar", "AAPL", "AAPL"},
		{"plain number", 42.0, 42.0},
		{"nil", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RawValue(tc.in)
			if got != tc.want {
				t.Errorf("RawValue(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestGetPath(t *testing.T) {
	tree := map[string]interface{}{
		"summaryDetail": map[string]interface{}{
			"trailingPE": map[string]interface{}{"raw": 28.5, "fmt": "28.50"},
		},
	}

	got := GetPath(tree, "summaryDetail", "trailingPE")
	want := map[string]interface{}{"raw": 28.5, "fmt": "28.50"}
	gotMap, ok := got.(map[string]interface{})
	if !ok || gotMap["raw"] != want["raw"] {
		t.Errorf("GetPath = %v, want %v", got, want)
	}

	if got := GetPath(tree, "summaryDetail", "missingKey"); got != nil {
		t.Errorf("GetPath missing key = %v, want nil", got)
	}
	if got := GetPath(tree, "summaryDetail", "trailingPE", "raw"); got != 28.5 {
		t.Errorf("GetPath descending into non-map = %v, want nil-ish failure path", got)
	}
	if got := GetPath(tree, "notThere"); got != nil {
		t.Errorf("GetPath on absent root key = %v, want nil", got)
	}
}

func TestField(t *testing.T) {
	m := map[string]interface{}{
		"regularMarketPrice": map[string]interface{}{"raw": 150.23, "fmt": "150.23"},
	}
	if got := Field(m, "regularMarketPrice"); got != 150.23 {
		t.Errorf("Field = %v, want 150.23", got)
	}
	if got := Field(nil, "regularMarketPrice"); got != nil {
		t.Errorf("Field(nil, ...) = %v, want nil", got)
	}
	if got := Field(m, "missing"); got != nil {
		t.Errorf("Field(missing key) = %v, want nil", got)
	}
}

func TestAsFloat(t *testing.T) {
	cases := []struct {
		in      interface{}
		want    float64
		wantOk  bool
	}{
		{3.14, 3.14, true},
		{7, 7.0, true},
		{int64(9), 9.0, true},
		{"2.5", 2.5, true},
		{"not a number", 0, false},
		{true, 0, false},
		{nil, 0, false},
	}
	for _, tc := range cases {
		got, ok := AsFloat(tc.in)
		if ok != tc.wantOk || (ok && got != tc.want) {
			t.Errorf("AsFloat(%v) = (%v, %v), want (%v, %v)", tc.in, got, ok, tc.want, tc.wantOk)
		}
	}
}

func TestAsInt64(t *testing.T) {
	got, ok := AsInt64(42.9)
	if !ok || got != 42 {
		t.Errorf("AsInt64(42.9) = (%v, %v), want (42, true)", got, ok)
	}
	if _, ok := AsInt64("nope"); ok {
		t.Errorf("AsInt64(\"nope\") ok = true, want false")
	}
}

func TestAsString(t *testing.T) {
	cases := []struct {
		in   interface{}
		want string
	}{
		{nil, ""},
		{"AAPL", "AAPL"},
		{150.5, "150.5"},
		{true, "true"},
		{42, "42"},
	}
	for _, tc := range cases {
		if got := AsString(tc.in); got != tc.want {
			t.Errorf("AsString(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEpochToDate(t *testing.T) {
	if got := EpochToDate(1700000000); got != "2023-11-14" {
		t.Errorf("EpochToDate(1700000000) = %q, want 2023-11-14", got)
	}
	if got := EpochToDate("not a number"); got != "" {
		t.Errorf("EpochToDate(invalid) = %q, want empty string", got)
	}
}

func TestEpochToDateTimeNY(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tzdata not available: %v", err)
	}
	got := EpochToDateTimeNY(1700000000, loc)
	want := time.Unix(1700000000, 0).In(loc).Format("2006-01-02 15:04:05-07:00")
	if got != want {
		t.Errorf("EpochToDateTimeNY = %q, want %q", got, want)
	}

	gotUTC := EpochToDateTimeNY(1700000000, nil)
	wantUTC := time.Unix(1700000000, 0).UTC().Format("2006-01-02 15:04:05-07:00")
	if gotUTC != wantUTC {
		t.Errorf("EpochToDateTimeNY(nil loc) = %q, want %q", gotUTC, wantUTC)
	}

	if got := EpochToDateTimeNY("bad", loc); got != "" {
		t.Errorf("EpochToDateTimeNY(invalid) = %q, want empty string", got)
	}
}

func TestEpochToMidnight(t *testing.T) {
	if got := EpochToMidnight(1700000000); got != "2023-11-14 00:00:00" {
		t.Errorf("EpochToMidnight = %q, want 2023-11-14 00:00:00", got)
	}
	if got := EpochToMidnight("bad"); got != "" {
		t.Errorf("EpochToMidnight(invalid) = %q, want empty string", got)
	}
}

func TestDateStringToMidnight(t *testing.T) {
	if got := DateStringToMidnight("2023-11-14"); got != "2023-11-14 00:00:00" {
		t.Errorf("DateStringToMidnight = %q, want 2023-11-14 00:00:00", got)
	}
	if got := DateStringToMidnight(""); got != "" {
		t.Errorf("DateStringToMidnight(\"\") = %q, want empty string", got)
	}
}

func TestParseDateOnly(t *testing.T) {
	got, err := ParseDateOnly("2023-11-14")
	if err != nil {
		t.Fatalf("ParseDateOnly error: %v", err)
	}
	want := time.Date(2023, time.November, 14, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("ParseDateOnly = %v, want %v", got, want)
	}
	if _, err := ParseDateOnly("14/11/2023"); err == nil {
		t.Error("ParseDateOnly(\"14/11/2023\") expected error, got nil")
	}
}
