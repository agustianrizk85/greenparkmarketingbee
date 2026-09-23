package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WAR ROOM MARKETING — satu layar kesehatan belanja iklan untuk CEO.
//
// PEMBAGIAN TUGAS yang dijaga ketat, sama dengan war room Permit dan
// Perencanaan: SELURUH ANGKA dihitung di sini, AI hanya menyusun kalimat dan
// usulan di atasnya. Layar tetap berguna walau AI mati.
//
// SUMBERNYA DUA, KEDUANYA MILIK MARKETING SENDIRI: iklan Meta (menu Iklan/Ads,
// lewat metaapi) dan Alur Konten (basis data modul ini). Tidak ada angka divisi
// lain, dan tidak ada kotak masuk WhatsApp/Instagram — layar ini soal UANG IKLAN
// dan KESIAPAN MATERINYA, bukan soal percakapan.
//
// Akibat yang disengaja dari batas itu: stok unit, booking, dan akad TIDAK ada
// di layar ini, karena ketiganya milik Sales. Artinya war room ini bisa bilang
// "uang ini terbuang" tapi tidak bisa bilang "barangnya sudah habis" — dan itu
// ditulis apa adanya di celah_data supaya tidak ada yang mengira layar ini sudah
// menimbang ketersediaan unit.
//
// KENAPA DIHITUNG DI BACKEND, BUKAN DI LAYAR. Angka war room memindahkan uang
// sungguhan: "hentikan kampanye ini" berarti belanja harian berhenti. Kalau
// layar menghitung sendiri, dua tab yang umurnya berbeda bisa menampilkan dua
// angka untuk kampanye yang sama, dan AI menerima angka ketiga.
//
// KENAPA DISARING DI SINI JUGA. Lingkup "semua proyek" dan lingkup satu proyek
// memakai RUMUS YANG SAMA — datanya saja yang disaring lebih dulu. Menyaring di
// layar akan melahirkan rumus kedua, dan rumus kedua selalu menyimpang.

// SumberWarRoom adalah pintu ke data iklan di metaapi (ringkasan kampanye, tren
// harian & naskah iklan, dan peta proyek→akun iklan). Antarmuka, bukan pemanggilan HTTP langsung, supaya
// perhitungan bisa diuji tanpa metaapi hidup — dan supaya kegagalan satu sumber
// tidak pernah merobohkan seluruh layar.
type SumberWarRoom interface {
	Iklan(ctx context.Context, token string) (IklanMentah, error)
	IklanRinci(ctx context.Context, token string) (IklanRinciMentah, error)
	ProyekMeta(ctx context.Context, token string) ([]ProyekMeta, error)
}

// HitungKeputusan mengembalikan ringkasan perintah war room yang tercatat.
type HitungKeputusan func(ctx context.Context) (WRKeputusanRingkas, error)

// HitungKonten membaca keadaan produksi konten dari basis data Marketing
// sendiri (menu Alur Konten). Dipisah dari SumberWarRoom karena datanya lokal —
// tidak ada HTTP, tidak ada token, dan tidak pernah gagal karena jaringan.
type HitungKonten func(ctx context.Context) (KeadaanKonten, error)

// WarRoomService menyusun muatan war room.
type WarRoomService struct {
	sumber    SumberWarRoom
	keputusan HitungKeputusan
	konten    HitungKonten
	aturan    WRAturan
	rentang   string
	sekarang  func() time.Time

	mu        sync.Mutex
	singgahan map[string]singgahan
}

// singgahan menyimpan muatan yang baru saja dihitung, per lingkup.
type singgahan struct {
	wr   WarRoom
	pada time.Time
}

// batasBaca memotong penantian ke metaapi.
//
// Graph API Meta kadang menjawab dalam belasan detik. Tanpa batas ini layar
// menggantung di "memuat…" tanpa satu pun angka — padahal angka konten dan
// perintah sudah siap sejak milidetik pertama. Lewat batas ini, sumber yang
// lambat berubah jadi dimensi abu beserta alasannya, dan layarnya tetap terbit.
const batasBaca = 12 * time.Second

// umurSinggahan: muatan war room dipakai ulang selama satu menit.
//
// Layar menyegarkan diri tiap menit DAN tiap ada dorongan realtime, sedangkan
// angka iklan Meta berubah dalam hitungan jam. Tanpa singgahan, tiap dorongan
// memukul Graph API lagi — lambat untuk yang menunggu, dan boros kuota untuk
// semua orang.
const umurSinggahan = 60 * time.Second

// Batas panjang daftar. Layar hanya sanggup dibaca sekilas, dan konteks AI
// dibatasi 14.000 karakter — daftar panjang mengorbankan keduanya sekaligus.
const (
	maksKampanye = 8
	maksKreatif  = 6
	maksLead     = 8
	maksProyek   = 12
	maksKonten   = 8
	maksTren     = 30
)

// NewWarRoomService merakit layanannya. keputusan dan konten boleh nil (dibaca
// sebagai "belum ada datanya").
func NewWarRoomService(s SumberWarRoom, keputusan HitungKeputusan, konten HitungKonten, aturan WRAturan, rentang string) *WarRoomService {
	if rentang == "" {
		rentang = "30d"
	}
	return &WarRoomService{
		sumber: s, keputusan: keputusan, konten: konten,
		aturan: lengkapiAturan(aturan), rentang: rentang, sekarang: time.Now,
		singgahan: map[string]singgahan{},
	}
}

// SetJam mengganti sumber waktu (dipakai uji).
func (s *WarRoomService) SetJam(f func() time.Time) { s.sekarang = f }

// lengkapiAturan mengisi ambang yang belum diatur dan menuliskan penjelasannya.
//
// AmbangBelajarBelanja diturunkan dari target biaya per hasil, bukan angka
// bulat tersendiri: maknanya "kampanye ini belum sempat menghasilkan tiga hasil
// pun, jadi belum pantas dinilai". Diturunkan begitu, ia ikut bergerak saat
// targetnya diubah — ambang tetap akan diam-diam menjadi salah.
func lengkapiAturan(a WRAturan) WRAturan {
	if a.BiayaPerHasilTarget <= 0 {
		a.BiayaPerHasilTarget = 150000
	}
	if a.FrekuensiMaks <= 0 {
		a.FrekuensiMaks = 3
	}
	if a.CTRMinPersen <= 0 {
		a.CTRMinPersen = 1
	}
	if a.JamBalasLeadMaks <= 0 {
		a.JamBalasLeadMaks = 2
	}
	if a.AmbangBelajarBelanja <= 0 {
		a.AmbangBelajarBelanja = 3 * a.BiayaPerHasilTarget
	}
	a.Sumber = SumberManual
	a.Penjelasan = []string{
		"Target biaya per hasil dan ambang lain diketik manual, bukan dihitung dari data.",
		"Ambang belajar = 3 kali target biaya per hasil; kampanye di bawahnya belum pantas dinilai, apalagi dihentikan.",
		"Frekuensi di atas ambang berarti orang yang sama melihat iklan itu terlalu sering.",
	}
	return a
}

// celahTetap selalu ikut: batas bacaan war room ini perlu disebut setiap kali,
// bukan sekali di dokumentasi yang tidak dibaca siapa pun saat rapat.
var celahTetap = []string{
	"Layar ini hanya membaca iklan Meta dan Alur Konten. Percakapan WhatsApp/Instagram tidak dihitung di sini.",
	"Stok unit, booking, dan akad milik Sales dan tidak dibaca layar ini — usulan budget di sini belum menimbang ketersediaan unit.",
}

// Susun membaca semua sumber lalu menghitung seluruh muatan war room.
//
// Sumber yang mati TIDAK menggagalkan permintaan: dimensinya menjadi abu dan
// alasannya masuk celah_data. War room yang padam total saat satu layanan
// ngambek jauh lebih merugikan daripada war room yang jujur bilang bagian mana
// yang sedang buta.
func (s *WarRoomService) Susun(ctx context.Context, token string, l WRLingkup) WarRoom {
	now := s.sekarang()
	if wr, ada := s.dariSinggahan(l, now); ada {
		return wr
	}
	// Penantian dibatasi di sini, bukan hanya di klien HTTP: yang dijaga adalah
	// waktu terbitnya layar, bukan waktu satu panggilan.
	ctx, batal := context.WithTimeout(ctx, batasBaca)
	defer batal()
	celah := []string{}

	// Ketiga bacaan metaapi berjalan BERSAMAAN. Berurutan, satu panggilan yang
	// lambat menambah waktu tunggu semua yang lain, dan war room adalah layar
	// yang ditunggu orang sambil berdiri.
	var (
		iklan                         IklanMentah
		rinci                         IklanRinciMentah
		proyekMeta                    []ProyekMeta
		errIklan, errRinci, errProyek error
		wg                            sync.WaitGroup
	)
	wg.Add(3)
	go func() { defer wg.Done(); iklan, errIklan = s.sumber.Iklan(ctx, token) }()
	go func() { defer wg.Done(); rinci, errRinci = s.sumber.IklanRinci(ctx, token) }()
	go func() { defer wg.Done(); proyekMeta, errProyek = s.sumber.ProyekMeta(ctx, token) }()
	wg.Wait()

	if errIklan != nil {
		celah = append(celah, "Data iklan tidak bisa dibaca: "+errIklan.Error())
	} else if !iklan.Configured {
		ket := "Akun iklan Meta belum tersambung."
		if iklan.Error != "" {
			ket = "Akun iklan Meta bermasalah: " + iklan.Error
		}
		celah = append(celah, ket)
	}
	if errRinci != nil {
		celah = append(celah, "Tren harian dan naskah iklan tidak bisa dibaca: "+errRinci.Error())
	}
	if errProyek != nil {
		celah = append(celah, "Peta proyek ke akun iklan tidak bisa dibaca: "+errProyek.Error())
	}

	saring := saringanDari(l, proyekMeta)
	wr := WarRoom{
		Divisi:         "marketing",
		TanggalData:    now.Format(time.RFC3339),
		Lingkup:        WRLingkupMeta{ProyekID: l.ProyekID, Nama: saring.nama},
		ProyekTersedia: pilihanProyek(proyekMeta),
		Aturan:         s.aturan,
	}
	if l.ProyekID != "" && !saring.ketemu {
		celah = append(celah, "Proyek yang dipilih tidak ada di peta proyek; angka di layar kembali ke seluruh proyek.")
	}

	wr.Iklan, wr.KampanyeBoros, wr.KampanyeTerbaik = s.hitungIklan(iklan, saring)
	wr.Iklan.Rentang = s.rentang
	wr.KreatifLelah = s.hitungKreatif(rinci)
	wr.Tren = potongTren(rinci)
	wr.Proyek = s.hitungProyek(proyekMeta, iklan, saring)

	if s.konten != nil {
		k, err := s.konten(ctx)
		if err != nil {
			celah = append(celah, "Keadaan alur konten tidak bisa dibaca: "+err.Error())
		} else {
			wr.Konten = k.Ringkas
			wr.KontenTersendat = potong(k.Tersendat, maksKonten)
			wr.Orang = potong(k.Orang, maksOrang)
		}
	}
	if s.keputusan != nil {
		if k, err := s.keputusan(ctx); err == nil {
			wr.Keputusan = k
		} else {
			celah = append(celah, "Catatan keputusan war room tidak bisa dibaca: "+err.Error())
		}
	}

	wr.Corong = corong(wr)
	wr.StatusSistem = s.status(wr)
	wr.CelahData = append(append(celah, celahDariStatus(wr.StatusSistem)...), celahTetap...)
	s.simpanSinggahan(l, wr, now)
	return wr
}

// dariSinggahan mengembalikan muatan yang belum basi untuk lingkup ini.
func (s *WarRoomService) dariSinggahan(l WRLingkup, now time.Time) (WarRoom, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ada := s.singgahan[l.ProyekID]
	if !ada || now.Sub(c.pada) > umurSinggahan {
		return WarRoom{}, false
	}
	return c.wr, true
}

func (s *WarRoomService) simpanSinggahan(l WRLingkup, wr WarRoom, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.singgahan == nil {
		s.singgahan = map[string]singgahan{}
	}
	s.singgahan[l.ProyekID] = singgahan{wr: wr, pada: now}
}

/* ---- penyaring lingkup ---------------------------------------------------- */

// saringan memegang referensi akun milik SATU proyek. Kosong = seluruh proyek.
type saringan struct {
	aktif  bool
	ketemu bool
	nama   string
	iklan  map[string]bool // id/nama akun iklan
	wa     map[string]bool // phone_number_id
	ig     map[string]bool // page id / username
}

func saringanDari(l WRLingkup, proyek []ProyekMeta) saringan {
	s := saringan{nama: "Semua proyek", iklan: map[string]bool{}, wa: map[string]bool{}, ig: map[string]bool{}}
	if strings.TrimSpace(l.ProyekID) == "" {
		return s
	}
	for _, p := range proyek {
		if strconv.Itoa(p.ID) != strings.TrimSpace(l.ProyekID) {
			continue
		}
		s = saringanProyek(p)
		s.aktif, s.ketemu = true, true
	}
	return s
}

// saringanProyek merakit penyaring dari akun-akun satu proyek. Dipakai dua
// tempat — penyaring lingkup dan hitungan per proyek — supaya "kampanye milik
// proyek ini" hanya punya satu definisi.
func saringanProyek(p ProyekMeta) saringan {
	s := saringan{aktif: true, ketemu: true, nama: p.Name, iklan: map[string]bool{}, wa: map[string]bool{}, ig: map[string]bool{}}
	for _, a := range p.Accounts {
		ref := strings.ToLower(strings.TrimSpace(a.Ref))
		if ref == "" {
			continue
		}
		switch a.Kind {
		case "ad":
			s.iklan[ref] = true
		case "wa":
			s.wa[ref] = true
		case "ig":
			s.ig[ref] = true
		}
	}
	return s
}

func (s saringan) lolosIklan(akunID, akun string) bool {
	if !s.aktif {
		return true
	}
	return s.iklan[strings.ToLower(akunID)] || s.iklan[strings.ToLower(akun)]
}

func pilihanProyek(proyek []ProyekMeta) []WRPilihan {
	out := make([]WRPilihan, 0, len(proyek))
	for _, p := range proyek {
		out = append(out, WRPilihan{ID: strconv.Itoa(p.ID), Nama: p.Name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Nama < out[j].Nama })
	return out
}

/* ---- iklan ---------------------------------------------------------------- */

// hitungIklan menjumlah kampanye yang lolos saringan, lalu memisahkan yang
// boros dan yang terbaik.
//
// Totalnya dihitung ulang dari kampanye — TIDAK memakai "totals" bawaan metaapi
// — supaya lingkup "semua" dan lingkup satu proyek memakai rumus yang sama.
func (s *WarRoomService) hitungIklan(m IklanMentah, f saringan) (WRIklan, []WRKampanye, []WRKampanye) {
	out := WRIklan{Sumber: SumberMeta, Terpasang: m.Configured}
	boros := []WRKampanye{}
	bagus := []WRKampanye{}

	for _, c := range m.Campaigns {
		if !f.lolosIklan(c.AccountID, c.Account) {
			continue
		}
		out.Belanja += c.Spend
		out.Hasil += c.Results
		out.Impresi += c.Impressions
		out.Klik += c.Clicks
		out.JangkauanTerjumlah += c.Reach
		out.FrekuensiTertinggi = math.Max(out.FrekuensiTertinggi, c.Frequency)
		if strings.EqualFold(c.EffectiveStatus, "ACTIVE") || strings.EqualFold(c.Status, "ACTIVE") {
			out.KampanyeAktif++
		}
		if c.Issues > 0 {
			out.KampanyeBermasalah++
		}

		k := s.nilaiKampanye(c)
		if k.Masalah != "" {
			// Uang yang dipertaruhkan = belanja kampanye itu sendiri, jadi
			// daftarnya diurutkan dari yang paling mahal. Yang di bawah ambang
			// belajar tetap ikut supaya terlihat, hanya ditandai.
			boros = append(boros, k)
			if !k.DiBawahAmbangBelajar {
				out.BelanjaBoros += c.Spend
			}
		} else if c.Results > 0 {
			bagus = append(bagus, k)
		}
	}

	out.BiayaPerHasil = bagi(out.Belanja, out.Hasil)
	out.CTR = persen(out.Klik, out.Impresi)
	out.CPC = bagi(out.Belanja, out.Klik)
	out.PersenBelanjaBoros = persen(out.BelanjaBoros, out.Belanja)

	sort.Slice(boros, func(i, j int) bool { return boros[i].Belanja > boros[j].Belanja })
	sort.Slice(bagus, func(i, j int) bool { return bagus[i].BiayaPerHasil < bagus[j].BiayaPerHasil })
	return out, potong(boros, maksKampanye), potong(bagus, 3)
}

// nilaiKampanye menerjemahkan satu kampanye jadi baris war room beserta
// masalahnya. Urutan pemeriksaannya menentukan masalah mana yang disebut: yang
// paling mahal akibatnya lebih dulu.
func (s *WarRoomService) nilaiKampanye(c KampanyeMentah) WRKampanye {
	k := WRKampanye{
		ID: c.ID, Nama: c.Name, Akun: c.Account, Status: c.EffectiveStatus,
		Belanja: c.Spend, Hasil: c.Results, BiayaPerHasil: bagi(c.Spend, c.Results),
		CTR: c.CTR, Frekuensi: c.Frequency, Sumber: SumberMeta,
		DiBawahAmbangBelajar: c.Spend < s.aturan.AmbangBelajarBelanja,
	}
	switch {
	case c.Spend > 0 && c.Results == 0:
		k.Masalah = "belanja jalan tanpa satu pun hasil"
	case k.BiayaPerHasil > 2*s.aturan.BiayaPerHasilTarget:
		k.Masalah = "biaya per hasil lebih dari dua kali target"
	case k.BiayaPerHasil > s.aturan.BiayaPerHasilTarget:
		k.Masalah = "biaya per hasil di atas target"
	case c.Frequency >= s.aturan.FrekuensiMaks:
		k.Masalah = "orang yang sama terlalu sering melihat iklan ini"
	case c.CTR > 0 && c.CTR < s.aturan.CTRMinPersen:
		k.Masalah = "iklan jarang diklik"
	}
	return k
}

// hitungKreatif menandai naskah iklan yang sudah lelah: sudah banyak dilihat
// tapi mahal atau jarang diklik. Kreatif TIDAK punya frekuensi sendiri di data
// metaapi, jadi kelelahannya dibaca dari biaya dan CTR-nya.
func (s *WarRoomService) hitungKreatif(m IklanRinciMentah) []WRKreatif {
	out := []WRKreatif{}
	for i, c := range m.Creatives {
		masalah := ""
		switch {
		case c.Spend >= s.aturan.AmbangBelajarBelanja && c.Results == 0:
			masalah = "sudah menghabiskan belanja tanpa hasil"
		case c.CostPerResult > 2*s.aturan.BiayaPerHasilTarget:
			masalah = "biaya per hasil lebih dari dua kali target"
		case c.CTR > 0 && c.CTR < s.aturan.CTRMinPersen:
			masalah = "jarang diklik dibanding ambang"
		}
		if masalah == "" {
			continue
		}
		judul := strings.TrimSpace(c.Title)
		if judul == "" {
			judul = ringkas(c.Body, 80)
		}
		out = append(out, WRKreatif{
			ID: "kreatif-" + strconv.Itoa(i+1), Judul: judul,
			Belanja: c.Spend, Hasil: c.Results, BiayaPerHasil: c.CostPerResult,
			CTR: c.CTR, Impresi: c.Impressions, Iklan: c.Ads,
			Masalah: masalah, Sumber: SumberMeta,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Belanja > out[j].Belanja })
	return potong(out, maksKreatif)
}

func potongTren(m IklanRinciMentah) []WRTitikTren {
	out := make([]WRTitikTren, 0, len(m.Daily))
	for _, d := range m.Daily {
		out = append(out, WRTitikTren{Tanggal: d.Date, Belanja: d.Spend, Hasil: d.Results, Klik: d.Clicks, Impresi: d.Impressions})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tanggal < out[j].Tanggal })
	if len(out) > maksTren {
		out = out[len(out)-maksTren:]
	}
	return out
}

/* ---- proyek --------------------------------------------------------------- */

// hitungProyek memecah belanja iklan dan lead menurut PROYEK, memakai peta
// proyek→akun yang dikelola di menu Akun Meta.
//
// Inilah yang membuat usulan "alihkan budget dari proyek A ke proyek B" punya
// dasar tanpa menyentuh data Sales: dua proyek dibandingkan lewat belanja dan
// biaya per hasilnya sendiri.
func (s *WarRoomService) hitungProyek(proyek []ProyekMeta, ik IklanMentah, f saringan) []WRProyek {
	out := []WRProyek{}
	for _, p := range proyek {
		fp := saringanProyek(p)
		// Saat lingkupnya satu proyek, proyek lain tidak ikut ditampilkan —
		// layar dan AI harus melihat cakupan yang sama persis.
		if f.aktif && !strings.EqualFold(fp.nama, f.nama) {
			continue
		}
		baris := WRProyek{ID: strconv.Itoa(p.ID), Nama: p.Name, Sumber: SumberMeta}
		for _, c := range ik.Campaigns {
			if !fp.lolosIklan(c.AccountID, c.Account) {
				continue
			}
			baris.Belanja += c.Spend
			baris.Hasil += c.Results
			baris.Kampanye++
		}
		baris.BiayaPerHasil = bagi(baris.Belanja, baris.Hasil)
		out = append(out, baris)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Belanja > out[j].Belanja })
	return potong(out, maksProyek)
}

/* ---- corong & status ------------------------------------------------------ */

// corong merangkai perjalanan uang dari belanja sampai percakapan yang dibalas.
//
// Berhenti di "dibalas", bukan di akad: akad milik Sales dan layar ini sengaja
// tidak membacanya. Tahap terakhir yang benar-benar diketahui Marketing adalah
// percakapan yang sudah dijawab timnya.
// corong merangkai perjalanan uang dari belanja sampai hasil yang dilaporkan
// Meta.
//
// Berhenti di situ, bukan di percakapan atau akad: keduanya bukan data layar
// ini. Menambahkan tahap yang angkanya tidak dibaca hanya melahirkan corong yang
// ekornya selalu nol, dan nol yang bukan kenyataan lebih buruk daripada tahap
// yang memang tidak ditampilkan.
func corong(w WarRoom) []WRTahapCorong {
	return []WRTahapCorong{
		{Tahap: "belanja iklan", Jumlah: w.Iklan.Belanja, Satuan: "rupiah", Sumber: SumberMeta},
		{Tahap: "impresi", Jumlah: w.Iklan.Impresi, Satuan: "tayangan", BiayaPerSatuan: bagi(w.Iklan.Belanja, w.Iklan.Impresi), Sumber: SumberMeta},
		{Tahap: "klik", Jumlah: w.Iklan.Klik, Satuan: "klik", BiayaPerSatuan: w.Iklan.CPC, KonversiPersen: persen(w.Iklan.Klik, w.Iklan.Impresi), Sumber: SumberMeta},
		{Tahap: "hasil iklan", Jumlah: w.Iklan.Hasil, Satuan: "hasil", BiayaPerSatuan: w.Iklan.BiayaPerHasil, KonversiPersen: persen(w.Iklan.Hasil, w.Iklan.Klik), Sumber: SumberMeta},
	}
}

// status menilai tiap dimensi. Ambangnya ditulis di aturan dan dikirim ke layar
// maupun AI, jadi warna di sini selalu bisa dipertanggungjawabkan.
func (s *WarRoomService) status(w WarRoom) WRStatus {
	st := WRStatus{}

	// Belanja iklan: seberapa besar bagian belanja yang mengalir ke kampanye
	// bermasalah. Dihitung dari uang, bukan dari jumlah kampanye — sepuluh
	// kampanye receh yang jelek tidak sepenting satu kampanye besar yang jelek.
	switch {
	case !w.Iklan.Terpasang || w.Iklan.Belanja == 0:
		st.BelanjaIklan = WarnaAbu
	case w.Iklan.PersenBelanjaBoros >= 30:
		st.BelanjaIklan = WarnaMerah
	case w.Iklan.PersenBelanjaBoros >= 10:
		st.BelanjaIklan = WarnaKuning
	default:
		st.BelanjaIklan = WarnaHijau
	}

	switch {
	case !w.Iklan.Terpasang || w.Iklan.Belanja == 0:
		st.MutuLead = WarnaAbu
	case w.Iklan.Hasil == 0:
		st.MutuLead = WarnaMerah
	case w.Iklan.BiayaPerHasil > 2*s.aturan.BiayaPerHasilTarget:
		st.MutuLead = WarnaMerah
	case w.Iklan.BiayaPerHasil > s.aturan.BiayaPerHasilTarget:
		st.MutuLead = WarnaKuning
	default:
		st.MutuLead = WarnaHijau
	}

	// Produksi konten: pekerjaan Marketing sendiri. Tidak ada pekerjaan berjalan
	// = abu, bukan hijau — papan kosong bisa berarti timnya belum mencatat apa
	// pun, dan itu keadaan yang perlu diketahui, bukan disamarkan jadi "aman".
	switch {
	case w.Konten.Pekerjaan == 0:
		st.ProduksiKonten = WarnaAbu
	case w.Konten.Terlambat >= 3:
		st.ProduksiKonten = WarnaMerah
	case w.Konten.Terlambat > 0 || w.Konten.SegeraJatuhTempo > 0:
		st.ProduksiKonten = WarnaKuning
	default:
		st.ProduksiKonten = WarnaHijau
	}

	st.Umum = terburuk(st.BelanjaIklan, st.MutuLead, st.ProduksiKonten)
	return st
}

// terburuk memilih warna paling gawat. Abu TIDAK dianggap gawat maupun aman: ia
// hanya menang kalau tidak ada satu pun dimensi yang benar-benar terbaca —
// sebab "semua datanya kosong" memang keadaan yang berbeda dari "semuanya baik".
func terburuk(w ...string) string {
	peringkat := map[string]int{WarnaHijau: 1, WarnaKuning: 2, WarnaMerah: 3}
	best, ada := WarnaHijau, false
	for _, x := range w {
		n, ok := peringkat[x]
		if !ok {
			continue
		}
		ada = true
		if n > peringkat[best] {
			best = x
		}
	}
	if !ada {
		return WarnaAbu
	}
	return best
}

func celahDariStatus(st WRStatus) []string {
	out := []string{}
	if st.BelanjaIklan == WarnaAbu {
		out = append(out, "Belanja iklan belum terbaca — usulan soal budget tidak punya dasar.")
	}
	if st.ProduksiKonten == WarnaAbu {
		out = append(out, "Belum ada konten berjalan di Alur Konten — kesiapan materi iklan tidak terpantau.")
	}
	return out
}

/* ---- pembantu ------------------------------------------------------------- */

func bagi(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return bulat1(a / b)
}

func persen(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return bulat1(a / b * 100)
}

func bulat1(f float64) float64 { return math.Round(f*10) / 10 }

func potong[T any](s []T, n int) []T {
	if len(s) > n {
		return s[:n]
	}
	if s == nil {
		return []T{}
	}
	return s
}

func ringkas(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// jamSejak mengubah waktu pesan terakhir menjadi lama menunggu dalam jam.
// Waktu yang tidak terbaca menghasilkan 0, bukan angka besar: menuduh sebuah
// percakapan terbengkalai berjam-jam gara-gara format tanggal yang aneh akan
// memindahkan perhatian tim ke tempat yang salah.
func jamSejak(iso string, now time.Time) float64 {
	if strings.TrimSpace(iso) == "" {
		return 0
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, iso); err == nil {
			d := now.Sub(t).Hours()
			if d < 0 {
				return 0
			}
			return d
		}
	}
	return 0
}

// RingkasUntukLog dipakai handler menulis satu baris log yang bisa dibaca saat
// menelusuri keluhan "angka di layar aneh".
func RingkasUntukLog(w WarRoom) string {
	return fmt.Sprintf("warroom marketing: %s | belanja %.0f, hasil %.0f, boros %.1f%%, konten telat %d",
		w.StatusSistem.Umum, w.Iklan.Belanja, w.Iklan.Hasil, w.Iklan.PersenBelanjaBoros, w.Konten.Terlambat)
}
