package service

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"marketingflow/internal/model"
	"marketingflow/internal/repository"
)

// KEADAAN ALUR KONTEN untuk War Room — dimensi keempat layar, plus performa
// orang-orangnya.
//
// Datanya milik Marketing sendiri: work item dan langkahnya, persis yang
// ditampilkan menu Alur Konten dan peringatan dini di Ringkasan. Tidak ada
// panggilan keluar, tidak ada token.
//
// KENAPA MENUMPANG EarlyWarnings UNTUK YANG TERSENDAT. Aturan "terlambat" dan
// "mendekati tenggat" sudah ditegakkan di sana dan sudah dibaca tim tiap hari di
// layar Ringkasan. Menulis rumus kedua di sini akan melahirkan dua versi
// "terlambat" yang berbeda di dua layar — dan war room adalah tempat paling
// buruk untuk menemukan perbedaan itu.

// KeadaanKonten adalah seluruh bacaan Alur Konten untuk satu kali susun.
//
// Dikembalikan sebagai satu struct, bukan tiga nilai: ketiganya dihitung dari
// bacaan basis data YANG SAMA, dan memisahkannya jadi tiga pemanggilan berarti
// tiga kali membaca tabel yang sama untuk satu layar.
type KeadaanKonten struct {
	Ringkas   WRKonten
	Tersendat []WRKontenItem
	Orang     []WROrang
}

// KontenDari merakit pembaca keadaan konten dari peringatan dini, daftar item,
// langkah, dan akun.
func KontenDari(dash *DashboardService, item *repository.WorkItemRepository, langkah *repository.StepRepository, akun *repository.UserRepository) HitungKonten {
	return func(_ context.Context) (KeadaanKonten, error) {
		out := KeadaanKonten{Ringkas: WRKonten{Sumber: SumberKonten}}

		items, err := item.List()
		if err != nil {
			return out, err
		}
		out.Ringkas.Pekerjaan = len(items)

		peringatan, err := dash.EarlyWarnings()
		if err != nil {
			return out, err
		}
		out.Ringkas.LangkahTerbuka = len(peringatan)

		daftar := []WRKontenItem{}
		for _, p := range peringatan {
			tingkat := ""
			switch p.Severity {
			case SeverityCritical:
				out.Ringkas.Terlambat++
				tingkat = WarnaMerah
			case SeverityWarning:
				out.Ringkas.SegeraJatuhTempo++
				tingkat = WarnaKuning
			case SeverityInfo:
				// Butuh masukan (mis. budget belum diisi) dihitung, tapi tidak
				// ikut daftar tersendat: ia menunggu isian, bukan menunggu orang
				// mengerjakan — dan mencampurnya akan menenggelamkan yang benar-
				// benar macet.
				out.Ringkas.ButuhMasukan++
				continue
			}
			tenggat := ""
			if p.DueDate != nil {
				tenggat = p.DueDate.Format(time.RFC3339)
			}
			daftar = append(daftar, WRKontenItem{
				ID:      strconv.FormatUint(uint64(p.WorkItemID), 10),
				Judul:   p.WorkItemTitle,
				Langkah: p.StepName,
				Pemilik: p.Owner,
				Tingkat: tingkat,
				Pesan:   p.Message,
				Tenggat: tenggat,
				Sumber:  SumberKonten,
			})
		}
		sortKonten(daftar)
		out.Tersendat = daftar

		orang, err := performaOrang(langkah, akun)
		if err != nil {
			return out, err
		}
		out.Orang = orang
		return out, nil
	}
}

// jendelaHasil — sepanjang apa "yang sudah dituntaskan" dihitung ke belakang.
// Sebulan: cukup panjang untuk menangkap irama kerja konten (satu kampanye
// biasanya selesai dalam hitungan minggu), cukup pendek untuk tidak membuat
// orang yang sudah dua bulan menganggur tetap terlihat sibuk.
const jendelaHasil = 30 * 24 * time.Hour

// maksOrang — berapa baris performa yang dibawa ke layar.
const maksOrang = 10

// performaOrang menyusun beban dan hasil kerja PER PERAN PIC.
//
// KENAPA PER PERAN, BUKAN PER AKUN. Langkah alur konten dimiliki PERAN
// (Copywriter, Design Grafis, Talent & Videografer…), bukan user id — begitulah
// katalog alurnya dirancang, karena yang memegang peran bisa berganti tanpa
// mengubah alur kerjanya. Nama akunnya dilampirkan bila ada yang memegang peran
// itu, jadi layar tetap menyebut orang, bukan jabatan kosong.
//
// KENAPA BEBAN DAN HASIL SEKALIGUS. Menilai orang dari sisa pekerjaannya saja
// membuat yang paling produktif terlihat paling buruk — ia kebagian paling
// banyak justru karena cepat. Karena itu tiap baris membawa keduanya: yang
// menggantung DAN yang sudah dituntaskan sebulan terakhir.
func performaOrang(langkah *repository.StepRepository, akun *repository.UserRepository) ([]WROrang, error) {
	if langkah == nil {
		return []WROrang{}, nil
	}
	terbuka, err := langkah.OpenSteps()
	if err != nil {
		return nil, err
	}
	selesai, err := langkah.LangkahSelesaiSejak(time.Now().Add(-jendelaHasil))
	if err != nil {
		return nil, err
	}

	per := map[string]*WROrang{}
	ambil := func(peran string) *WROrang {
		peran = strings.TrimSpace(peran)
		if peran == "" {
			peran = "(tanpa PIC)"
		}
		if o, ada := per[peran]; ada {
			return o
		}
		o := &WROrang{Peran: peran, Sumber: SumberKonten}
		per[peran] = o
		return o
	}

	now := time.Now()
	for _, s := range terbuka {
		o := ambil(s.Owner)
		o.LangkahAktif++
		if s.DueDate == nil {
			continue
		}
		switch {
		case now.After(*s.DueDate):
			o.Terlambat++
		case s.DueDate.Sub(now) <= 24*time.Hour:
			o.SegeraJatuhTempo++
		}
	}
	for _, s := range selesai {
		o := ambil(s.Owner)
		o.Selesai30Hari++
		if s.CompletedAt == nil {
			continue
		}
		t := s.CompletedAt.Format(time.RFC3339)
		if o.TerakhirSelesai == nil || t > *o.TerakhirSelesai {
			o.TerakhirSelesai = &t
		}
	}

	pemegang := pemegangPeran(akun)
	out := make([]WROrang, 0, len(per))
	for _, o := range per {
		o.Nama = pemegang[strings.ToLower(o.Peran)]
		out = append(out, *o)
	}
	// Yang paling banyak terlambat lebih dulu — itu yang perlu ditanyakan di
	// rapat; sisanya urut menurut beban.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Terlambat != out[j].Terlambat {
			return out[i].Terlambat > out[j].Terlambat
		}
		if out[i].LangkahAktif != out[j].LangkahAktif {
			return out[i].LangkahAktif > out[j].LangkahAktif
		}
		return out[i].Peran < out[j].Peran
	})
	return potong(out, maksOrang), nil
}

// pemegangPeran memetakan nama peran ke nama akun yang memegangnya.
//
// Pencocokannya longgar (saling memuat) karena label peran di katalog alur dan
// jabatan di akun tidak selalu ditulis sama persis — "Talent" di akun vs
// "Talent & Videografer" di langkah. Lebih baik menyebut nama yang mungkin
// benar daripada membiarkan seluruh kolom nama kosong.
func pemegangPeran(akun *repository.UserRepository) map[string]string {
	out := map[string]string{}
	if akun == nil {
		return out
	}
	daftar, err := akun.List()
	if err != nil {
		return out
	}
	for _, u := range daftar {
		pos := strings.ToLower(strings.TrimSpace(u.Position))
		if pos == "" || u.Role == model.RoleViewer {
			continue
		}
		if _, ada := out[pos]; !ada {
			out[pos] = u.Name
		}
	}
	// Peran yang tidak persis sama tetap dicocokkan lewat saling-memuat.
	cocok := map[string]string{}
	for pos, nama := range out {
		cocok[pos] = nama
	}
	for pos, nama := range out {
		for pos2 := range out {
			if pos != pos2 && strings.Contains(pos2, pos) {
				if _, ada := cocok[pos2]; !ada {
					cocok[pos2] = nama
				}
			}
		}
	}
	return cocok
}

// sortKonten menaruh yang merah lebih dulu, lalu yang tenggatnya paling lama
// terlewat.
func sortKonten(d []WRKontenItem) {
	sort.SliceStable(d, func(i, j int) bool { return lebihMendesak(d[i], d[j]) })
}

func lebihMendesak(a, b WRKontenItem) bool {
	if a.Tingkat != b.Tingkat {
		return a.Tingkat == WarnaMerah
	}
	if a.Tenggat == "" {
		return false
	}
	if b.Tenggat == "" {
		return true
	}
	return a.Tenggat < b.Tenggat
}
