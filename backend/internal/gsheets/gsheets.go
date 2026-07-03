// Package gsheets fetches sheet data from a Google Spreadsheet. When a service
// account credential is supplied it uses the Sheets API (private sheets OK);
// otherwise it downloads the public XLSX export (the sheet must be link-viewable).
// Either way every tab is returned as raw string rows so the content-plan parser
// processes a live Google Sheet the same way it would an uploaded workbook.
package gsheets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
	"golang.org/x/oauth2/google"
)

const apiBase = "https://sheets.googleapis.com/v4/spreadsheets/"

// Client reads Google Sheets with a service-account credential.
type Client struct {
	cred []byte
}

// New loads the service-account JSON bytes. Empty input returns (nil, nil) — the
// sync feature then falls back to the public XLSX export path.
func New(cred []byte) (*Client, error) {
	if len(cred) == 0 {
		return nil, nil
	}
	return &Client{cred: cred}, nil
}

// FetchAll returns every tab of the spreadsheet as raw string rows, keyed by the
// tab title.
func (c *Client) FetchAll(ctx context.Context, spreadsheetID string) (map[string][][]string, error) {
	conf, err := google.JWTConfigFromJSON(c.cred, "https://www.googleapis.com/auth/spreadsheets.readonly")
	if err != nil {
		return nil, fmt.Errorf("kredensial Google tidak valid: %w", err)
	}
	httpClient := conf.Client(ctx)
	httpClient.Timeout = 120 * time.Second

	titles, err := fetchTitles(httpClient, spreadsheetID)
	if err != nil {
		return nil, err
	}
	if len(titles) == 0 {
		return nil, fmt.Errorf("spreadsheet tidak punya tab/sheet")
	}

	u := apiBase + url.PathEscape(spreadsheetID) +
		"/values:batchGet?valueRenderOption=UNFORMATTED_VALUE&dateTimeRenderOption=SERIAL_NUMBER"
	for _, t := range titles {
		u += "&ranges=" + url.QueryEscape("'"+t+"'")
	}
	var resp struct {
		ValueRanges []struct {
			Values [][]interface{} `json:"values"`
		} `json:"valueRanges"`
	}
	if err := getJSON(httpClient, u, &resp); err != nil {
		return nil, err
	}

	out := make(map[string][][]string, len(titles))
	for i, t := range titles {
		if i >= len(resp.ValueRanges) {
			break
		}
		out[t] = toStrings(resp.ValueRanges[i].Values)
	}
	return out, nil
}

var sheetIDRe = regexp.MustCompile(`/spreadsheets/d/([a-zA-Z0-9-_]+)`)

// ParseSheetID extracts the spreadsheet id from a full Google Sheets URL, or
// returns the input unchanged when it already looks like a bare id.
func ParseSheetID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if m := sheetIDRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	if !strings.ContainsAny(s, "/ ?#") {
		return s
	}
	return ""
}

// FetchPublicXLSX downloads a spreadsheet via its public XLSX export endpoint (no
// credentials) and returns every tab as raw string rows. It works only when the
// spreadsheet is shared "anyone with the link can view" (or published).
func FetchPublicXLSX(ctx context.Context, spreadsheetID string) (map[string][][]string, error) {
	exportURL := "https://docs.google.com/spreadsheets/d/" + url.PathEscape(spreadsheetID) + "/export?format=xlsx"
	// Google's export endpoint is flaky for non-browser clients: it intermittently
	// returns an HTML interstitial (which is not a zip) instead of the workbook.
	// A browser-like User-Agent plus a few retries makes it reliable.
	client := &http.Client{Timeout: 120 * time.Second}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 600 * time.Millisecond):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, exportURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
		req.Header.Set("Accept", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,*/*")

		res, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("akses spreadsheet gagal: %w", err)
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(res.Body, 200<<20))
		res.Body.Close()

		if res.StatusCode != http.StatusOK {
			// 401/403/404 mean the sheet isn't link-viewable — retrying won't help.
			if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusNotFound {
				return nil, fmt.Errorf("spreadsheet belum publik (HTTP %d) — buka Share → 'Siapa saja dengan link' → 'Pelihat', lalu coba lagi", res.StatusCode)
			}
			lastErr = fmt.Errorf("spreadsheet tidak bisa diakses (HTTP %d), mencoba ulang…", res.StatusCode)
			continue
		}
		// A real .xlsx is a zip — it starts with the "PK" magic bytes. Anything
		// else (an HTML interstitial / consent page) means retry.
		if !hasZipMagic(body) {
			ct := res.Header.Get("Content-Type")
			if strings.Contains(ct, "text/html") && looksLikePermissionWall(body) {
				return nil, fmt.Errorf("spreadsheet belum publik — buka Share → 'Siapa saja dengan link' → 'Pelihat', lalu coba lagi")
			}
			lastErr = fmt.Errorf("Google membalas non-xlsx (kemungkinan interstitial), mencoba ulang…")
			continue
		}
		return ReadXLSX(body)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("gagal mengambil spreadsheet setelah beberapa percobaan")
	}
	return nil, lastErr
}

// hasZipMagic reports whether the bytes begin with a ZIP local-file header,
// which every .xlsx workbook does ("PK\x03\x04").
func hasZipMagic(b []byte) bool {
	return len(b) >= 4 && b[0] == 'P' && b[1] == 'K' && (b[2] == 0x03 || b[2] == 0x05 || b[2] == 0x07)
}

// looksLikePermissionWall detects Google's "you need access / sign in" HTML so a
// genuine permission problem is reported instead of being retried pointlessly.
func looksLikePermissionWall(b []byte) bool {
	n := len(b)
	if n > 4000 {
		n = 4000
	}
	s := strings.ToLower(string(b[:n]))
	return strings.Contains(s, "request access") || strings.Contains(s, "sign in") ||
		strings.Contains(s, "you need access") || strings.Contains(s, "accounts.google.com")
}

// ReadXLSX parses raw .xlsx bytes into per-tab string rows. Exposed so the same
// canonical reading (RawCellValue: serial dates, en-US decimals) can be reused
// for uploaded files and tests.
func ReadXLSX(body []byte) (map[string][][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gagal membaca workbook (pastikan URL benar & sheet bisa diakses): %w", err)
	}
	defer f.Close()

	out := make(map[string][][]string)
	for _, name := range f.GetSheetList() {
		rows, err := f.GetRows(name, excelize.Options{RawCellValue: true})
		if err != nil {
			continue
		}
		out[name] = rows
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("spreadsheet kosong / tidak ada tab terbaca")
	}
	return out, nil
}

func fetchTitles(client *http.Client, id string) ([]string, error) {
	var meta struct {
		Sheets []struct {
			Properties struct {
				Title string `json:"title"`
			} `json:"properties"`
		} `json:"sheets"`
	}
	u := apiBase + url.PathEscape(id) + "?fields=sheets.properties.title"
	if err := getJSON(client, u, &meta); err != nil {
		return nil, err
	}
	titles := make([]string, 0, len(meta.Sheets))
	for _, s := range meta.Sheets {
		titles = append(titles, s.Properties.Title)
	}
	return titles, nil
}

func getJSON(client *http.Client, u string, dst interface{}) error {
	res, err := client.Get(u)
	if err != nil {
		return fmt.Errorf("akses Google Sheets gagal: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 200<<20))
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Google Sheets HTTP %d: %s", res.StatusCode, shorten(string(body)))
	}
	return json.Unmarshal(body, dst)
}

func toStrings(rows [][]interface{}) [][]string {
	out := make([][]string, len(rows))
	for i, r := range rows {
		row := make([]string, len(r))
		for j, v := range r {
			row[j] = cellToString(v)
		}
		out[i] = row
	}
	return out
}

func cellToString(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "TRUE"
		}
		return "FALSE"
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return x.String()
	default:
		return fmt.Sprintf("%v", x)
	}
}

func shorten(s string) string {
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}
