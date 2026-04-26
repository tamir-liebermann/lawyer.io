// Package realestatedata queries the public data.gov.il datastore for
// Israeli real estate transactions.
//
// Primary endpoint: datastore_search_sql (LIKE query by FULLADRESS).
// Fallback endpoint: datastore_search with q= full-text search.
//
// Dataset resource ID: 5c78e9fa-c2e2-4771-93ff-7f400a12f7ba
package realestatedata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultBaseURL    = "https://data.gov.il/api/3/action/datastore_search"
	DefaultSQLBaseURL = "https://data.gov.il/api/3/action/datastore_search_sql"
	DefaultResourceID = "5c78e9fa-c2e2-4771-93ff-7f400a12f7ba"
	DefaultLimit      = 500
)

// cityAliases maps colloquial Hebrew city names to the official name stored
// in the data.gov.il dataset. Tel Aviv is stored as "תל אביב יפו" (merged
// municipality), not the commonly typed "תל אביב".
var cityAliases = map[string]string{
	"תל אביב":     "תל אביב יפו",
	"תל-אביב":     "תל אביב יפו",
	"תל אביב-יפו": "תל אביב יפו",
	"פתח תקוה":    "פתח תקווה",
}

// resolveCity returns the canonical dataset city name, or the input unchanged.
func resolveCity(city string) string {
	if canonical, ok := cityAliases[city]; ok {
		return canonical
	}
	return city
}

type cacheEntry struct {
	records   []Record
	expiresAt time.Time
}

// Fetcher queries the data.gov.il datastore.
type Fetcher struct {
	baseURL        string
	sqlBaseURL     string
	resourceID     string
	limit          int
	http           *http.Client
	lookbackMonths int
	now            func() time.Time

	cacheMu  sync.Mutex
	cache    map[string]cacheEntry
	cacheTTL time.Duration
}

// Option configures a Fetcher.
type Option func(*Fetcher)

func WithBaseURL(u string) Option        { return func(f *Fetcher) { f.baseURL = u } }
func WithSQLBaseURL(u string) Option     { return func(f *Fetcher) { f.sqlBaseURL = u } }
func WithResourceID(id string) Option    { return func(f *Fetcher) { f.resourceID = id } }
func WithLimit(n int) Option             { return func(f *Fetcher) { f.limit = n } }
func WithHTTPClient(h *http.Client) Option { return func(f *Fetcher) { f.http = h } }
func WithLookbackMonths(n int) Option    { return func(f *Fetcher) { f.lookbackMonths = n } }
func WithNow(fn func() time.Time) Option { return func(f *Fetcher) { f.now = fn } }
func WithCacheTTL(d time.Duration) Option { return func(f *Fetcher) { f.cacheTTL = d } }

// New constructs a Fetcher with sensible defaults.
func New(opts ...Option) *Fetcher {
	f := &Fetcher{
		baseURL:        DefaultBaseURL,
		sqlBaseURL:     DefaultSQLBaseURL,
		resourceID:     DefaultResourceID,
		limit:          DefaultLimit,
		http:           &http.Client{Timeout: 10 * time.Second},
		lookbackMonths: 24, // extended; dataset has 2-3 month reporting lag
		now:            time.Now,
		cache:          make(map[string]cacheEntry),
		cacheTTL:       10 * time.Minute,
	}
	for _, o := range opts {
		o(f)
	}
	return f
}

// Record is a normalized transaction record.
type Record struct {
	City     string
	Price    float64   // nominal, ILS
	Rooms    float64   // e.g. 3, 3.5
	AreaSqm  float64
	DealDate time.Time // parsed; zero if unparseable
}

// PricePerSqm returns price/AreaSqm or 0 if either is zero.
func (r Record) PricePerSqm() float64 {
	if r.AreaSqm == 0 {
		return 0
	}
	return r.Price / r.AreaSqm
}

type rawResult struct {
	Success bool `json:"success"`
	Result  struct {
		Records []map[string]interface{} `json:"records"`
	} `json:"result"`
}

// sqlQueryTemplate is used by fetchSQL. %s = resource_id, %s = city fragment, %d = limit.
const sqlQueryTemplate = `SELECT * FROM "%s" WHERE "FULLADRESS" LIKE '%%%s%%' LIMIT %d`

// FetchByCity returns normalized records for the given Hebrew city name,
// restricted to the configured lookback window. It resolves city aliases,
// checks the in-memory TTL cache, then tries the SQL endpoint (LIKE query)
// before falling back to the generic q= full-text approach.
func (f *Fetcher) FetchByCity(ctx context.Context, city string) ([]Record, error) {
	city = strings.TrimSpace(city)
	if city == "" {
		return nil, fmt.Errorf("realestatedata: empty city")
	}

	canonical := resolveCity(city)

	if recs, ok := f.fromCache(canonical); ok {
		log.Printf("realestatedata: cache hit for %q (%d records)", canonical, len(recs))
		return recs, nil
	}

	recs, err := f.fetchSQL(ctx, canonical)
	if err != nil {
		log.Printf("realestatedata: SQL fetch failed for %q: %v; falling back to q=", canonical, err)
		recs, err = f.fetchQ(ctx, canonical)
		if err != nil {
			return nil, err
		}
	}

	log.Printf("realestatedata: raw records for %q: %d", canonical, len(recs))

	cutoff := f.now().AddDate(0, -f.lookbackMonths, 0)
	out := f.filterRecords(recs, city, canonical, cutoff)

	f.toCache(canonical, out)
	return out, nil
}

// filterRecords applies city, date, and price filters with detailed logging.
// It accepts both the original city name and the canonical alias so that test
// data using the short form (e.g., "תל אביב") still matches.
func (f *Fetcher) filterRecords(recs []Record, original, canonical string, cutoff time.Time) []Record {
	out := make([]Record, 0, len(recs))
	noCityMatch, noPrice := 0, 0
	for _, r := range recs {
		if r.City != "" &&
			!strings.Contains(r.City, canonical) &&
			!strings.Contains(r.City, original) {
			noCityMatch++
			continue
		}
		if !r.DealDate.IsZero() && r.DealDate.Before(cutoff) {
			continue
		}
		if r.Price <= 0 {
			noPrice++
			continue
		}
		out = append(out, r)
	}
	log.Printf("realestatedata: filter stats for %q — cityMismatch=%d noPrice=%d kept=%d (cutoff=%s)",
		canonical, noCityMatch, noPrice, len(out), cutoff.Format("2006-01-02"))
	return out
}

func (f *Fetcher) fetchSQL(ctx context.Context, canonical string) ([]Record, error) {
	if f.sqlBaseURL == "" {
		return nil, fmt.Errorf("realestatedata: SQL base URL not set")
	}
	sqlStr := fmt.Sprintf(sqlQueryTemplate, f.resourceID, canonical, f.limit)
	q := url.Values{}
	q.Set("sql", sqlStr)
	u := f.sqlBaseURL + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("realestatedata: new SQL request: %w", err)
	}
	return f.doFetch(req)
}

func (f *Fetcher) fetchQ(ctx context.Context, canonical string) ([]Record, error) {
	q := url.Values{}
	q.Set("resource_id", f.resourceID)
	q.Set("limit", strconv.Itoa(f.limit))
	q.Set("q", canonical)
	u := f.baseURL + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("realestatedata: new request: %w", err)
	}
	return f.doFetch(req)
}

func (f *Fetcher) doFetch(req *http.Request) ([]Record, error) {
	resp, err := f.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("realestatedata: http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("realestatedata: read: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("realestatedata: status=%d body=%s", resp.StatusCode, string(body))
	}

	var raw rawResult
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("realestatedata: decode: %w", err)
	}
	if !raw.Success {
		return nil, fmt.Errorf("realestatedata: API returned success=false")
	}

	out := make([]Record, 0, len(raw.Result.Records))
	for _, row := range raw.Result.Records {
		out = append(out, normalize(row))
	}
	return out, nil
}

func (f *Fetcher) fromCache(canonical string) ([]Record, bool) {
	f.cacheMu.Lock()
	defer f.cacheMu.Unlock()
	e, ok := f.cache[canonical]
	if !ok || f.now().After(e.expiresAt) {
		return nil, false
	}
	return e.records, true
}

func (f *Fetcher) toCache(canonical string, recs []Record) {
	f.cacheMu.Lock()
	defer f.cacheMu.Unlock()
	f.cache[canonical] = cacheEntry{
		records:   recs,
		expiresAt: f.now().Add(f.cacheTTL),
	}
}

// Summary collapses a record slice into the numbers we feed to Claude.
type Summary struct {
	City          string
	Count         int
	AvgPrice      float64
	MinPrice      float64
	MaxPrice      float64
	AvgPricePerM2 float64
}

// Summarize computes aggregate stats. Returns zero Summary if records empty.
func Summarize(city string, records []Record) Summary {
	if len(records) == 0 {
		return Summary{City: city}
	}
	s := Summary{City: city, Count: len(records)}
	var sumPrice, sumPPSM float64
	var pPSMCount int
	s.MinPrice = records[0].Price
	s.MaxPrice = records[0].Price
	for _, r := range records {
		sumPrice += r.Price
		if r.Price < s.MinPrice {
			s.MinPrice = r.Price
		}
		if r.Price > s.MaxPrice {
			s.MaxPrice = r.Price
		}
		if ppsm := r.PricePerSqm(); ppsm > 0 {
			sumPPSM += ppsm
			pPSMCount++
		}
	}
	s.AvgPrice = sumPrice / float64(len(records))
	if pPSMCount > 0 {
		s.AvgPricePerM2 = sumPPSM / float64(pPSMCount)
	}
	return s
}

// FormatSummaryHebrew renders a summary as a short Hebrew paragraph suitable
// for injection into the system prompt.
func FormatSummaryHebrew(s Summary) string {
	if s.Count == 0 {
		return fmt.Sprintf(
			"לא נמצאו עסקאות ב-%s בשנתיים האחרונות במאגר הציבורי (שים לב: יש פיגור דיווח של 2-3 חודשים).",
			s.City,
		)
	}
	return fmt.Sprintf(
		"נתוני עסקאות ב-%s (עד שנתיים אחרונות, פיגור דיווח 2-3 חודשים): "+
			"מספר עסקאות: %d; מחיר ממוצע: %s ₪; טווח: %s–%s ₪; מחיר ממוצע למ\"ר: %s ₪.",
		s.City,
		s.Count,
		formatILS(s.AvgPrice),
		formatILS(s.MinPrice),
		formatILS(s.MaxPrice),
		formatILS(s.AvgPricePerM2),
	)
}

// SummaryForCity is a convenience that matches the chat.RealEstateFetcher
// interface: fetch + summarize + format in one call.
func (f *Fetcher) SummaryForCity(ctx context.Context, city string) (string, error) {
	recs, err := f.FetchByCity(ctx, city)
	if err != nil {
		return "", err
	}
	return FormatSummaryHebrew(Summarize(city, recs)), nil
}

// --- helpers ---

// normalize tolerates the dataset's inconsistent field names.
// DEALNATURE is the deal-type description (e.g., "דירה"), not sqm — use ASSETNETAREA.
func normalize(row map[string]interface{}) Record {
	var r Record
	r.City = firstString(row, "שם_ישוב", "FULLADRESS", "YISHUV", "city")
	r.Price = firstFloat(row, "מחיר", "DEALAMOUNT", "price", "TransactionSum")
	r.Rooms = firstFloat(row, "חדרים", "ASSETROOMSNUM", "ROOMS", "rooms")
	r.AreaSqm = firstFloat(row, "שטח", "ASSETNETAREA", "area", "DealNatureArea")
	if d := firstString(row, "תאריך_עסקה", "DEALDATETIME", "dealDate", "date"); d != "" {
		r.DealDate = parseDate(d)
	}
	return r
}

func firstString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			switch s := v.(type) {
			case string:
				if s != "" {
					return s
				}
			case float64:
				return strconv.FormatFloat(s, 'f', -1, 64)
			}
		}
	}
	return ""
}

func firstFloat(m map[string]interface{}, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			switch x := v.(type) {
			case float64:
				return x
			case string:
				s := strings.ReplaceAll(x, ",", "")
				s = strings.TrimSpace(s)
				if f, err := strconv.ParseFloat(s, 64); err == nil {
					return f
				}
			}
		}
	}
	return 0
}

// parseDate accepts the most common representations found in the dataset.
func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"02/01/2006",
		"2/1/2006",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// formatILS renders a float with comma thousands separators and no decimals.
func formatILS(v float64) string {
	i := int64(v + 0.5)
	neg := i < 0
	if neg {
		i = -i
	}
	digits := []byte(strconv.FormatInt(i, 10))
	var out []byte
	for idx, ch := range digits {
		if idx > 0 && (len(digits)-idx)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(ch))
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// SortByDateDesc sorts records by DealDate descending.
func SortByDateDesc(rs []Record) {
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].DealDate.After(rs[j].DealDate) })
}
