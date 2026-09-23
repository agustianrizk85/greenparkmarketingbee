package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// PENGAMBILAN DATA WAR ROOM dari metaapi.
//
// KENAPA TIDAK MENYALIN DATANYA. Marketing tidak menyimpan satu pun angka
// iklan; ia membacanya saat dibutuhkan. Menyalin berarti punya salinan
// kedua yang bisa basi diam-diam — dan war room justru layar yang paling tidak
// boleh menampilkan angka basi tanpa ada yang tahu.
//
// KENAPA MEMAKAI TOKEN PEMANGGIL. Permintaan diteruskan dengan token orang yang
// sedang membuka layar, bukan dengan akun layanan. Dua akibat yang disengaja:
// tidak ada kredensial tambahan yang perlu disimpan di sini, dan hak baca tetap
// hak orang itu sendiri — kalau akun iklannya tidak boleh ia lihat, war room-nya
// juga tidak menampilkannya.
//
// KENAPA ADA BATAS WAKTU. Tiga panggilan berjalan bersamaan; satu yang
// menggantung tidak boleh menahan seluruh layar. Batas keras layarnya sendiri
// ada di service (batasBaca) — yang di sini cuma jaring terakhir.
// Yang gagal berubah jadi dimensi abu beserta alasannya (lihat Susun).

// SumberHTTP membaca data war room dari metaapi (iklan, WhatsApp, Instagram,
// dan peta proyek→akun).
type SumberHTTP struct {
	MetaBase string
	Rentang  string
	klien    *http.Client
}

// NewSumberHTTP merakit pembaca dengan batas waktu yang wajar untuk layar yang
// ditunggu orang: lebih baik satu blok abu daripada layar yang berputar lama.
func NewSumberHTTP(metaBase, rentang string) *SumberHTTP {
	return &SumberHTTP{
		MetaBase: strings.TrimRight(metaBase, "/"),
		Rentang:  rentang,
		klien:    &http.Client{Timeout: 20 * time.Second},
	}
}

func (s *SumberHTTP) ambil(ctx context.Context, token, url string, keluar any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := s.klien.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		// Badan galat dipotong: pesan panjang dari layanan lain akan tampil di
		// celah_data yang dibaca CEO, dan di sana yang berguna hanya intinya.
		b, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("%s menjawab %d %s", ringkasURL(url), res.StatusCode, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(res.Body).Decode(keluar)
}

// Iklan membaca ringkasan kampanye dari metaapi.
func (s *SumberHTTP) Iklan(ctx context.Context, token string) (IklanMentah, error) {
	var out IklanMentah
	err := s.ambil(ctx, token, s.MetaBase+"/api/meta/ads?range="+s.Rentang, &out)
	return out, err
}

// IklanRinci membaca tren harian dan naskah iklan.
func (s *SumberHTTP) IklanRinci(ctx context.Context, token string) (IklanRinciMentah, error) {
	var out IklanRinciMentah
	err := s.ambil(ctx, token, s.MetaBase+"/api/meta/ads/detail?range="+s.Rentang, &out)
	return out, err
}

// ProyekMeta membaca peta proyek → akun iklan/WA/IG (dikelola di Admin).
func (s *SumberHTTP) ProyekMeta(ctx context.Context, token string) ([]ProyekMeta, error) {
	var out struct {
		Projects []ProyekMeta `json:"projects"`
	}
	err := s.ambil(ctx, token, s.MetaBase+"/api/meta/projects", &out)
	return out.Projects, err
}

func ringkasURL(u string) string {
	if i := strings.Index(u, "/api/"); i >= 0 {
		return u[i:]
	}
	return u
}
