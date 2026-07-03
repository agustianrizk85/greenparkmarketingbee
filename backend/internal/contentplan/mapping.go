package contentplan

import (
	"math"
	"strings"
	"time"

	"marketingflow/internal/model"
)

// projectNames maps a project abbreviation (as it appears in a tab name) to its
// full marketing name. Keys are normalised (lower-case, no spaces).
var projectNames = map[string]string{
	"gp":      "Greenpark Group",
	"lhl":     "Le Hauz Limo",
	"verua":   "Vertihome Serua",
	"thp":     "The Hauz Pancoran Mas",
	"verlim3": "Vertihauz Limo 3",
	"thc":     "The Hauz Cilodong",
	"zhl":     "Z Hauz Limo",
	"versaw":  "Vertihauz Sawangan",
	"verser":  "Vertihome Serpong",
	"verbur":  "Vertihauz Cibubur",
	"thpj":    "The Hauz Premiere",
	"lhc":     "Le Hauz Cibubur",
}

// tabPrefixes are stripped (longest first) from a sheet title to recover the
// project abbreviation. The leading space on " Copywrite THC" is handled by the
// TrimSpace in tabProject.
var tabPrefixes = []string{
	"Copywrite tiktok ",
	"Copywrite ",
	"Calendar ",
	"Story ",
}

// tabKind classifies a sheet by what the parser should do with it.
type tabKind int

const (
	tabOther tabKind = iota
	tabCalendar
	tabCopywrite       // regular copywrite block (Judul/Brief/Caption)
	tabCopywriteTiktok // tiktok script block — same layout, alur defaults to D
)

// classifyTab returns the project abbreviation key, its full name and the kind.
func classifyTab(title string) (key, name string, kind tabKind) {
	t := strings.TrimSpace(title)
	low := strings.ToLower(t)
	switch {
	case strings.HasPrefix(low, "copywrite tiktok"):
		kind = tabCopywriteTiktok
	case strings.HasPrefix(low, "copywrite"):
		kind = tabCopywrite
	case strings.HasPrefix(low, "calendar"):
		kind = tabCalendar
	default:
		return "", "", tabOther // Story* and anything else are ignored
	}

	abbr := t
	for _, p := range tabPrefixes {
		if len(abbr) >= len(p) && strings.EqualFold(abbr[:len(p)], p) {
			abbr = abbr[len(p):]
			break
		}
	}
	key = normKey(abbr)
	name = projectNames[key]
	if name == "" {
		name = strings.TrimSpace(abbr) // unknown project: show the raw abbreviation
	}
	return key, name, kind
}

// normKey lower-cases and removes spaces so "Verlim 3" and "Verlim3" collapse.
func normKey(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), " ", "")
}

// normTitle normalises a content title for matching calendar idea ↔ copywrite
// judul: lower-case, collapse whitespace, drop a trailing done-mark and common
// punctuation noise.
func normTitle(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "✅", "")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

// alurForType maps a raw calendar content-type label to a marketing alur.
// Returns ok=false when the label carries no usable signal.
func alurForType(raw string) (model.Alur, bool) {
	s := strings.ToLower(raw)
	switch {
	case strings.Contains(s, "hardsell"):
		return model.AlurHardsell, true // A
	case strings.Contains(s, "carousel"):
		return model.AlurCarousel, true // C
	case strings.Contains(s, "photo"), strings.Contains(s, "singlepost"), strings.Contains(s, "single post"):
		return model.AlurCarousel, true // C — static organic
	case strings.Contains(s, "reels"), strings.Contains(s, "tiktok"), strings.Contains(s, "video"):
		return model.AlurReels, true // D
	case strings.Contains(s, "softsell"):
		return model.AlurReels, true // D — softsells are predominantly video skits
	default:
		return "", false
	}
}

// inferAlurFromBrief is the fallback when the calendar gives no type: read the
// brief/judul text for format cues.
func inferAlurFromBrief(text string) model.Alur {
	s := strings.ToLower(text)
	switch {
	case strings.Contains(s, "hardsell"):
		return model.AlurHardsell
	case strings.Contains(s, "carousel"), strings.Contains(s, "slide "), strings.Contains(s, "slide1"), strings.Contains(s, "per slide"):
		return model.AlurCarousel
	case strings.Contains(s, "reels"), strings.Contains(s, "shooting"), strings.Contains(s, "oneshoot"),
		strings.Contains(s, "one shoot"), strings.Contains(s, "footage"), strings.Contains(s, "video"), strings.Contains(s, "scene"):
		return model.AlurReels
	default:
		return model.AlurReels // department's most common output
	}
}

// contentTypeKeywords detect a calendar "type" cell.
var contentTypeKeywords = []string{
	"softsell", "hardsell", "carousel", "reels", "tiktok",
	"photos", "photo", "singlepost", "single post", "story",
}

func looksLikeType(s string) bool {
	low := strings.ToLower(s)
	for _, k := range contentTypeKeywords {
		if strings.Contains(low, k) {
			return true
		}
	}
	return false
}

// monthHeaderName returns the canonical Indonesian month name when the cell is a
// standalone month header (e.g. "MARET", "APRIL"), else "".
var idMonths = map[string]time.Month{
	"januari": time.January, "februari": time.February, "maret": time.March,
	"april": time.April, "mei": time.May, "juni": time.June, "juli": time.July,
	"agustus": time.August, "september": time.September, "oktober": time.October,
	"november": time.November, "desember": time.December,
}

func monthHeaderName(s string) string {
	t := normKey(s)
	if _, ok := idMonths[t]; ok {
		return t
	}
	return ""
}

// excelSerialToDate converts an Excel/Sheets serial day number to a date. The
// 1900 date system epoch is 1899-12-30 (accounting for Excel's fictional leap
// day). Returns the zero time for non-positive serials.
func excelSerialToDate(serial float64) time.Time {
	if serial <= 0 {
		return time.Time{}
	}
	epoch := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	days := int(math.Floor(serial))
	return epoch.AddDate(0, 0, days)
}
