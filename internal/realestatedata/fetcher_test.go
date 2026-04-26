package realestatedata

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeAPI returns a canned JSON response shaped like data.gov.il's endpoints.
// It accepts either a ?sql= param (SQL endpoint) or ?q= param (q= endpoint).
func fakeAPI(t *testing.T, records []map[string]interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hasSql := r.URL.Query().Get("sql") != ""
		hasQ := r.URL.Query().Get("q") != ""
		if !hasSql && !hasQ {
			t.Errorf("expected either sql or q query param, got neither")
		}
		io.Copy(io.Discard, r.Body)
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"result": map[string]interface{}{
				"records": records,
			},
		})
	}))
}

func fixedNow() time.Time {
	return time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
}

func TestFetchByCity_HappyPath(t *testing.T) {
	records := []map[string]interface{}{
		{"שם_ישוב": "תל אביב", "מחיר": float64(3200000), "שטח": float64(90), "חדרים": float64(4), "תאריך_עסקה": "2026-03-15"},
		{"שם_ישוב": "תל אביב", "מחיר": "2,800,000", "שטח": float64(70), "חדרים": float64(3), "תאריך_עסקה": "2026-02-01"},
		// out of 6-month window (keep WithLookbackMonths(6) so this stays outside)
		{"שם_ישוב": "תל אביב", "מחיר": float64(2500000), "שטח": float64(60), "חדרים": float64(3), "תאריך_עסקה": "2025-01-01"},
		// wrong city
		{"שם_ישוב": "חיפה", "מחיר": float64(2000000), "שטח": float64(80), "חדרים": float64(4), "תאריך_עסקה": "2026-04-01"},
		// zero price
		{"שם_ישוב": "תל אביב", "מחיר": float64(0), "שטח": float64(80), "חדרים": float64(4), "תאריך_עסקה": "2026-04-01"},
	}
	srv := fakeAPI(t, records)
	defer srv.Close()

	f := New(
		WithBaseURL(srv.URL),
		WithSQLBaseURL(srv.URL),
		WithNow(fixedNow),
		WithLookbackMonths(6),
	)

	recs, err := f.FetchByCity(context.Background(), "תל אביב")
	if err != nil {
		t.Fatalf("FetchByCity: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 records after filtering, got %d", len(recs))
	}
	if recs[0].Price != 3200000 {
		t.Errorf("first record price = %v", recs[0].Price)
	}
	if recs[1].Price != 2800000 {
		t.Errorf("second record price (string parsed) = %v", recs[1].Price)
	}
}

func TestFetchByCity_EmptyCity(t *testing.T) {
	f := New()
	_, err := f.FetchByCity(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty city")
	}
}

func TestFetchByCity_APIFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	// Both SQL and q= endpoints point at the failing server.
	f := New(WithBaseURL(srv.URL), WithSQLBaseURL(srv.URL))
	_, err := f.FetchByCity(context.Background(), "חיפה")
	if err == nil {
		t.Fatal("expected error on 500")
	}
}

func TestFetchByCity_SuccessFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"success":false}`))
	}))
	defer srv.Close()
	f := New(WithBaseURL(srv.URL), WithSQLBaseURL(srv.URL))
	_, err := f.FetchByCity(context.Background(), "חיפה")
	if err == nil {
		t.Fatal("expected error when success=false")
	}
	if !strings.Contains(err.Error(), "success=false") {
		t.Errorf("error should mention success=false, got: %v", err)
	}
}

func TestFetchByCity_TelAvivAlias(t *testing.T) {
	// Dataset stores Tel Aviv as "תל אביב יפו"; user types "תל אביב".
	records := []map[string]interface{}{
		{
			"FULLADRESS":   "תל אביב יפו רחוב דיזנגוף 1",
			"DEALAMOUNT":   float64(4000000),
			"ASSETNETAREA": float64(90),
			"DEALDATETIME": "2026-03-01T00:00:00",
		},
	}
	srv := fakeAPI(t, records)
	defer srv.Close()

	f := New(
		WithBaseURL(srv.URL),
		WithSQLBaseURL(srv.URL),
		WithNow(fixedNow),
		WithLookbackMonths(24),
	)
	recs, err := f.FetchByCity(context.Background(), "תל אביב")
	if err != nil {
		t.Fatalf("FetchByCity: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 record for Tel Aviv alias, got %d", len(recs))
	}
	if recs[0].Price != 4000000 {
		t.Errorf("price = %v", recs[0].Price)
	}
}

func TestFetchByCity_Cache(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		io.Copy(io.Discard, r.Body)
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"result":  map[string]interface{}{"records": []map[string]interface{}{}},
		})
	}))
	defer srv.Close()

	f := New(
		WithBaseURL(srv.URL),
		WithSQLBaseURL(srv.URL),
		WithNow(fixedNow),
		WithCacheTTL(10*time.Minute),
	)
	_, _ = f.FetchByCity(context.Background(), "חיפה")
	_, _ = f.FetchByCity(context.Background(), "חיפה")
	if callCount != 1 {
		t.Errorf("expected 1 HTTP call (second should hit cache), got %d", callCount)
	}
}

func TestSummarize_Aggregates(t *testing.T) {
	recs := []Record{
		{City: "תל אביב", Price: 3_000_000, AreaSqm: 100},
		{City: "תל אביב", Price: 4_000_000, AreaSqm: 100},
		{City: "תל אביב", Price: 2_000_000, AreaSqm: 50},
	}
	s := Summarize("תל אביב", recs)
	if s.Count != 3 {
		t.Errorf("Count = %d", s.Count)
	}
	if s.AvgPrice != 3_000_000 {
		t.Errorf("AvgPrice = %v", s.AvgPrice)
	}
	if s.MinPrice != 2_000_000 || s.MaxPrice != 4_000_000 {
		t.Errorf("min/max wrong: %v %v", s.MinPrice, s.MaxPrice)
	}
	// ppsm: 30000, 40000, 40000 => avg 36666.66...
	if s.AvgPricePerM2 < 36000 || s.AvgPricePerM2 > 37000 {
		t.Errorf("AvgPricePerM2 unexpected: %v", s.AvgPricePerM2)
	}
}

func TestSummarize_EmptyRecords(t *testing.T) {
	s := Summarize("חיפה", nil)
	if s.Count != 0 || s.City != "חיפה" {
		t.Errorf("unexpected zero summary: %+v", s)
	}
}

func TestFormatSummaryHebrew(t *testing.T) {
	s := Summary{City: "גבעתיים", Count: 10, AvgPrice: 3200000, MinPrice: 2800000, MaxPrice: 3700000, AvgPricePerM2: 35000}
	out := FormatSummaryHebrew(s)
	for _, want := range []string{"גבעתיים", "10", "3,200,000", "2,800,000", "3,700,000", "35,000"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in summary, got: %s", want, out)
		}
	}
}

func TestFormatSummaryHebrew_Empty(t *testing.T) {
	out := FormatSummaryHebrew(Summary{City: "חיפה"})
	if !strings.Contains(out, "לא נמצאו עסקאות") {
		t.Errorf("expected 'no transactions' message, got: %s", out)
	}
}

func TestSummaryForCity_EndToEnd(t *testing.T) {
	records := []map[string]interface{}{
		{"שם_ישוב": "רמת גן", "מחיר": float64(3000000), "שטח": float64(90), "תאריך_עסקה": "2026-03-10"},
		{"שם_ישוב": "רמת גן", "מחיר": float64(3500000), "שטח": float64(110), "תאריך_עסקה": "2026-02-10"},
	}
	srv := fakeAPI(t, records)
	defer srv.Close()

	f := New(WithBaseURL(srv.URL), WithSQLBaseURL(srv.URL), WithNow(fixedNow))
	out, err := f.SummaryForCity(context.Background(), "רמת גן")
	if err != nil {
		t.Fatalf("SummaryForCity: %v", err)
	}
	if !strings.Contains(out, "רמת גן") {
		t.Errorf("summary missing city name: %s", out)
	}
	if !strings.Contains(out, "2") {
		t.Errorf("summary should include count 2: %s", out)
	}
}

func TestNormalize_AlternateFieldNames(t *testing.T) {
	row := map[string]interface{}{
		"FULLADRESS":   "תל אביב יפו רחוב X",
		"DEALAMOUNT":   "2,500,000",
		"ASSETNETAREA": float64(75), // DEALNATURE is a string description, not area
		"DEALDATETIME": "2026-03-01T00:00:00",
	}
	r := normalize(row)
	if !strings.Contains(r.City, "תל אביב") {
		t.Errorf("city not picked up from FULLADRESS: %s", r.City)
	}
	if r.Price != 2_500_000 {
		t.Errorf("price not parsed from string: %v", r.Price)
	}
	if r.AreaSqm != 75 {
		t.Errorf("area not picked up from ASSETNETAREA: %v", r.AreaSqm)
	}
	if r.DealDate.Year() != 2026 {
		t.Errorf("date not parsed: %v", r.DealDate)
	}
}

func TestNormalize_AssetNetArea(t *testing.T) {
	// DEALNATURE is a deal-type description string — must NOT be parsed as area.
	row := map[string]interface{}{
		"FULLADRESS":   "חיפה רחוב הנמל 5",
		"DEALAMOUNT":   float64(2000000),
		"ASSETNETAREA": float64(80),
		"DEALNATURE":   "דירה", // string — should be ignored for area
		"DEALDATETIME": "2026-01-01T00:00:00",
	}
	r := normalize(row)
	if r.AreaSqm != 80 {
		t.Errorf("expected AreaSqm=80 from ASSETNETAREA, got %v", r.AreaSqm)
	}
}

func TestFormatILS_Thousands(t *testing.T) {
	cases := map[float64]string{
		0:          "0",
		123:        "123",
		1234:       "1,234",
		1234567:    "1,234,567",
		1234567.89: "1,234,568", // rounded
	}
	for in, want := range cases {
		if got := formatILS(in); got != want {
			t.Errorf("formatILS(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestParseDate_Formats(t *testing.T) {
	cases := []string{
		"2026-03-01T12:00:00Z",
		"2026-03-01T12:00:00",
		"2026-03-01",
		"01/03/2026",
	}
	for _, s := range cases {
		if parseDate(s).IsZero() {
			t.Errorf("parseDate(%q) returned zero time", s)
		}
	}
	if !parseDate("not-a-date").IsZero() {
		t.Error("parseDate should return zero for garbage input")
	}
}
