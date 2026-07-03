// Package contentplan parses the Marketing "Content Plan" Google Sheet into
// flat planned-content records that map onto WorkItems. The workbook has two
// relevant tab families per project:
//
//	Calendar <Proj>  — weekly grid; 3-row blocks of [day numbers] / [type] /
//	                   [idea]. Gives the content TYPE (→ alur) keyed by idea title.
//	Copywrite <Proj> — repeating field blocks (Tanggal Upload / Judul / Brief /
//	                   Caption) laid out in column groups. Gives the concrete,
//	                   captioned content pieces.
//
// Each copywrite item with a Judul becomes one Planned record; its alur is taken
// from the matching calendar type (joined by normalised title), falling back to
// a brief-text heuristic. Story tabs are ignored.
package contentplan

import (
	"crypto/sha1"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"

	"marketingflow/internal/model"
)

// Planned is one piece of planned content destined to become a WorkItem.
type Planned struct {
	Project     string      `json:"project"`      // full name, e.g. "Le Hauz Limo"
	ProjectKey  string      `json:"project_key"`  // normalised abbreviation, e.g. "lhl"
	Title       string      `json:"title"`        // Judul
	Alur        model.Alur  `json:"alur"`         // A/B/C/D
	ContentType string      `json:"content_type"` // raw calendar label when known
	Date        *time.Time  `json:"date"`         // planned upload date (may be nil)
	Brief       string      `json:"brief"`
	Caption     string      `json:"caption"`
	SourceTab   string      `json:"source_tab"`
	SourceKey   string      `json:"source_key"` // stable idempotency key
}

// Summary is the aggregate returned by a preview.
type Summary struct {
	TotalItems   int            `json:"total_items"`
	ByProject    map[string]int `json:"by_project"`
	ByAlur       map[string]int `json:"by_alur"`
	WithDate     int            `json:"with_date"`
	WithCaption  int            `json:"with_caption"`
	TabsSeen     int            `json:"tabs_seen"`
	TabsSkipped  []string       `json:"tabs_skipped"`
}

// Parse walks every tab and returns the planned content plus a summary.
func Parse(tabs map[string][][]string) ([]Planned, Summary) {
	// Stable tab order for deterministic output.
	names := make([]string, 0, len(tabs))
	for n := range tabs {
		names = append(names, n)
	}
	sort.Strings(names)

	// Pass 1: build per-project calendar type lookup (normTitle → raw type).
	calTypes := map[string]map[string]string{} // projectKey → title → type
	for _, name := range names {
		key, _, kind := classifyTab(name)
		if kind != tabCalendar {
			continue
		}
		m := calTypes[key]
		if m == nil {
			m = map[string]string{}
			calTypes[key] = m
		}
		parseCalendar(tabs[name], m)
	}

	// Pass 2: copywrite tabs → planned items.
	var out []Planned
	summary := Summary{ByProject: map[string]int{}, ByAlur: map[string]int{}}
	for _, name := range names {
		key, proj, kind := classifyTab(name)
		if kind == tabOther {
			if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "calendar") {
				summary.TabsSkipped = append(summary.TabsSkipped, name)
			}
			continue
		}
		summary.TabsSeen++
		if kind != tabCopywrite && kind != tabCopywriteTiktok {
			continue // calendar tabs already consumed in pass 1
		}
		items := parseCopywrite(tabs[name])
		for _, it := range items {
			if it.judul == "" {
				continue
			}
			p := Planned{
				Project:     proj,
				ProjectKey:  key,
				Title:       it.judul,
				ContentType: "",
				Date:        it.date,
				Brief:       it.brief,
				Caption:     it.caption,
				SourceTab:   name,
			}
			// Resolve alur.
			if kind == tabCopywriteTiktok {
				p.Alur = model.AlurReels
				p.ContentType = "TikTok"
			} else if raw, ok := calTypes[key][normTitle(it.judul)]; ok {
				if a, ok2 := alurForType(raw); ok2 {
					p.Alur = a
					p.ContentType = raw
				}
			}
			if p.Alur == "" {
				p.Alur = inferAlurFromBrief(it.judul + " " + it.brief)
			}
			p.SourceKey = makeSourceKey(p)
			out = append(out, p)

			summary.TotalItems++
			summary.ByProject[proj]++
			summary.ByAlur[string(p.Alur)]++
			if p.Date != nil {
				summary.WithDate++
			}
			if strings.TrimSpace(p.Caption) != "" {
				summary.WithCaption++
			}
		}
	}
	return out, summary
}

// makeSourceKey builds a stable idempotency key from project + date + title so a
// re-sync never duplicates an item.
func makeSourceKey(p Planned) string {
	date := ""
	if p.Date != nil {
		date = p.Date.Format("2006-01-02")
	}
	raw := strings.Join([]string{p.ProjectKey, date, normTitle(p.Title)}, "|")
	sum := sha1.Sum([]byte(raw))
	return "cp_" + hex.EncodeToString(sum[:8])
}

// --- calendar parsing ---

// parseCalendar scans a calendar grid and records normTitle→type. A type cell is
// any cell that looks like a content type; the idea (title) sits in the row
// directly below in the same column.
func parseCalendar(rows [][]string, out map[string]string) {
	for r := 0; r+1 < len(rows); r++ {
		for c := 0; c < len(rows[r]); c++ {
			cell := strings.TrimSpace(rows[r][c])
			if cell == "" || !looksLikeType(cell) {
				continue
			}
			if strings.Contains(strings.ToLower(cell), "story") {
				continue
			}
			var idea string
			if c < len(rows[r+1]) {
				idea = strings.TrimSpace(rows[r+1][c])
			}
			if idea == "" {
				continue
			}
			if nt := normTitle(idea); nt != "" {
				out[nt] = cell
			}
		}
	}
}

// --- copywrite parsing ---

type cwItem struct {
	judul   string
	brief   string
	caption string
	date    *time.Time
}

// field labels recognised in the left column of each copywrite block.
const (
	lblTanggal = "tanggal upload"
	lblJudul   = "judul"
	lblBrief   = "brief"
	lblCaption = "caption"
)

// parseCopywrite extracts items from a copywrite tab. Items are grouped by
// (month-block, label-column): every field of one item shares the same column,
// and a value sits in the cell immediately to the right of its label. Month
// header rows (a lone month name in column A) delimit the blocks so the first
// item of consecutive months don't merge.
func parseCopywrite(rows [][]string) []cwItem {
	// Month-block boundaries.
	var headerRows []int
	for r, row := range rows {
		if len(row) == 0 {
			continue
		}
		if monthHeaderName(row[0]) == "" {
			continue
		}
		// Header rows have the month only in column A; the rest is empty.
		lone := true
		for c := 1; c < len(row); c++ {
			if strings.TrimSpace(row[c]) != "" {
				lone = false
				break
			}
		}
		if lone {
			headerRows = append(headerRows, r)
		}
	}
	blockIndex := func(r int) int {
		idx := 0
		for _, hr := range headerRows {
			if hr <= r {
				idx++
			} else {
				break
			}
		}
		return idx
	}

	items := map[string]*cwItem{}
	var order []string
	get := func(blk, col int) *cwItem {
		key := strconv.Itoa(blk) + ":" + strconv.Itoa(col)
		it := items[key]
		if it == nil {
			it = &cwItem{}
			items[key] = it
			order = append(order, key)
		}
		return it
	}

	for r, row := range rows {
		for c := 0; c < len(row); c++ {
			label := strings.ToLower(strings.TrimSpace(row[c]))
			if label != lblTanggal && label != lblJudul && label != lblBrief && label != lblCaption {
				continue
			}
			val := ""
			if c+1 < len(row) {
				val = strings.TrimSpace(row[c+1])
			}
			if val == "" {
				continue
			}
			it := get(blockIndex(r), c)
			switch label {
			case lblTanggal:
				it.date = parseDateCell(val)
			case lblJudul:
				it.judul = val
			case lblBrief:
				it.brief = val
			case lblCaption:
				it.caption = val
			}
		}
	}

	res := make([]cwItem, 0, len(order))
	for _, k := range order {
		res = append(res, *items[k])
	}
	return res
}

// parseDateCell accepts an Excel serial (RawCellValue / Sheets SERIAL_NUMBER) or
// a textual dd/mm/yyyy date and returns the date, or nil when unparseable.
func parseDateCell(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		if t := excelSerialToDate(f); !t.IsZero() {
			return &t
		}
		return nil
	}
	for _, layout := range []string{"2/1/2006", "02/01/2006", "2-1-2006", "2006-01-02", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}
