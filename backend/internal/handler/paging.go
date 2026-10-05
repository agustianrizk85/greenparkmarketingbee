package handler

// Paging server untuk tabel dashboard. Kontraknya SAMA di semua backend dan
// dibaca apa adanya oleh DataTable mode server (dashboard/fe/src/lib/serverTable.ts):
//
//	GET …?page=1&limit=25&q=teks&sort=kolom&dir=asc|desc
//	→ {"items": [...], "total": N, "page": 1, "limit": 25}
//
// Tanpa `page` di query, handler tetap membalas bentuk lamanya — pemanggil lain
// (War Room, xdiv, antar-backend) tidak ikut berubah. Pakai mintaPaging untuk
// memutuskannya. Berkas ini disalin apa adanya ke tiap backend (pola yang sama
// dengan helper GP_FILE_DIR); kalau diubah, ubah semua salinannya.

import (
	"cmp"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

const (
	pagingLimitBawaan = 25
	pagingLimitMaks   = 500
)

// PageQuery adalah permintaan paging yang sudah dibersihkan.
type PageQuery struct {
	Page  int
	Limit int
	Q     string // sudah huruf kecil & dipangkas
	Sort  string
	Desc  bool
}

// PageResult adalah balasan paging; bentuk JSON-nya adalah kontrak FE.
type PageResult[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

// mintaPaging membaca query paging. ok=false berarti pemanggil TIDAK meminta
// paging (tidak ada `page`) → handler membalas bentuk lamanya.
func mintaPaging(r *http.Request) (PageQuery, bool) {
	v := r.URL.Query()
	if v.Get("page") == "" {
		return PageQuery{}, false
	}
	page, _ := strconv.Atoi(v.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(v.Get("limit"))
	if limit < 1 {
		limit = pagingLimitBawaan
	}
	if limit > pagingLimitMaks {
		limit = pagingLimitMaks
	}
	return PageQuery{
		Page:  page,
		Limit: limit,
		Q:     strings.ToLower(strings.TrimSpace(v.Get("q"))),
		Sort:  strings.TrimSpace(v.Get("sort")),
		Desc:  strings.EqualFold(v.Get("dir"), "desc"),
	}, true
}

// Kolom menjelaskan satu kolom yang bisa dicari/diurutkan: Teks dipakai untuk
// pencarian `q` dan pengurutan teks; Angka (bila diisi) dipakai untuk urutan
// numerik supaya "10" tidak jatuh sebelum "9".
type Kolom[T any] struct {
	Teks  func(T) string
	Angka func(T) float64
}

// halaman menyaring (q mencocokkan SEMUA kolom ber-Teks), mengurutkan (sort =
// nama kolom; tak dikenal → urutan asli), lalu memotong satu halaman.
func halaman[T any](rows []T, p PageQuery, kolom map[string]Kolom[T]) PageResult[T] {
	out := rows
	if p.Q != "" {
		kata := strings.Fields(p.Q)
		out = make([]T, 0, len(rows))
		for _, r := range rows {
			var sb strings.Builder
			for _, k := range kolom {
				if k.Teks != nil {
					sb.WriteString(strings.ToLower(k.Teks(r)))
					sb.WriteByte(' ')
				}
			}
			isi := sb.String()
			cocok := true
			for _, w := range kata {
				if !strings.Contains(isi, w) {
					cocok = false
					break
				}
			}
			if cocok {
				out = append(out, r)
			}
		}
	} else {
		out = slices.Clone(rows)
	}
	if k, ada := kolom[p.Sort]; ada {
		slices.SortStableFunc(out, func(a, b T) int {
			var c int
			if k.Angka != nil {
				c = cmp.Compare(k.Angka(a), k.Angka(b))
			} else if k.Teks != nil {
				c = bandingAlami(k.Teks(a), k.Teks(b))
			}
			if p.Desc {
				return -c
			}
			return c
		})
	}
	total := len(out)
	from := min((p.Page-1)*p.Limit, total)
	to := min(from+p.Limit, total)
	items := out[from:to]
	if items == nil {
		items = []T{}
	}
	return PageResult[T]{Items: items, Total: total, Page: p.Page, Limit: p.Limit}
}

// bandingAlami membandingkan teks tanpa peduli huruf besar/kecil, dengan deret
// angka dibandingkan sebagai ANGKA: "A2" < "A10", "B-9" < "B-10". Urutan teks
// biasa menaruh A10 sebelum A2 — salah untuk no. kavling, blok, nomor dokumen.
func bandingAlami(a, b string) int {
	a, b = strings.ToLower(a), strings.ToLower(b)
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ca, cb := a[i], b[j]
		if isDigit(ca) && isDigit(cb) {
			si := i
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			sj := j
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			na := strings.TrimLeft(a[si:i], "0")
			nb := strings.TrimLeft(b[sj:j], "0")
			if c := cmp.Compare(len(na), len(nb)); c != 0 {
				return c
			}
			if c := strings.Compare(na, nb); c != 0 {
				return c
			}
			continue
		}
		if ca != cb {
			return cmp.Compare(ca, cb)
		}
		i++
		j++
	}
	return cmp.Compare(len(a)-i, len(b)-j)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
