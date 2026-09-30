package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

// sumberUji adalah sumber palsu — seluruh perhitungan war room diuji tanpa
// metaapi hidup. Galat per sumber bisa dinyalakan satu per satu karena justru
// kegagalan sebagian itulah yang paling sering terjadi di lapangan, dan yang
// paling mudah salah ditangani.
type sumberUji struct {
	iklan    IklanMentah
	rinci    IklanRinciMentah
	proyek   []ProyekMeta
	errIklan error
}

func (s sumberUji) Iklan(context.Context, string) (IklanMentah, error) { return s.iklan, s.errIklan }
func (s sumberUji) IklanRinci(context.Context, string) (IklanRinciMentah, error) {
	return s.rinci, nil
}
func (s sumberUji) ProyekMeta(context.Context, string) ([]ProyekMeta, error) { return s.proyek, nil }

var jamUji = time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)

func layananUji(t *testing.T, s sumberUji) *WarRoomService {
	t.Helper()
	return layananKonten(t, s, nil)
}

func layananKonten(t *testing.T, s sumberUji, konten HitungKonten) *WarRoomService {
	t.Helper()
	svc := NewWarRoomService(s, nil, konten, WRAturan{BiayaPerHasilTarget: 100000}, "30d")
	svc.SetJam(func() time.Time { return jamUji })
	return svc
}

// kampanye merakit satu baris kampanye metaapi dengan angka yang ikut
// menentukan penilaian; sisanya (impresi, klik, jangkauan) diisi nilai wajar
// karena tidak satu pun uji di berkas ini bergantung padanya.
func kampanye(id, nama, akun string, belanja, hasil, ctr, frek float64) KampanyeMentah {
	return KampanyeMentah{
		ID: id, Name: nama, Account: akun, AccountID: akun,
		Status: "ACTIVE", EffectiveStatus: "ACTIVE",
		Spend: belanja, Results: hasil, CTR: ctr, Frequency: frek,
		Impressions: 10000, Clicks: 100, Reach: 5000,
	}
}

func iklanUji() IklanMentah {
	m := IklanMentah{Configured: true}
	// Boros: 1 juta tanpa satu pun hasil (jauh di atas ambang belajar 300rb).
	m.Campaigns = append(m.Campaigns, kampanye("c1", "Hardsell Mawar", "akun-a", 1_000_000, 0, 1.5, 2))
	// Sehat: 10 hasil dengan biaya 50rb, di bawah target 100rb.
	m.Campaigns = append(m.Campaigns, kampanye("c2", "Reels Melati", "akun-b", 500_000, 10, 2.0, 1.5))
	// Baru jalan: 100rb, belum pantas dinilai.
	m.Campaigns = append(m.Campaigns, kampanye("c3", "Uji Kreatif Baru", "akun-a", 100_000, 0, 1.2, 1))
	return m
}

func proyekUji() []ProyekMeta {
	return []ProyekMeta{
		{ID: 7, Name: "GP Mawar", Accounts: []AkunProyekMeta{{Kind: "ad", Ref: "akun-b"}, {Kind: "wa", Ref: "wa1"}}},
		{ID: 8, Name: "GP Melati", Accounts: []AkunProyekMeta{{Kind: "ad", Ref: "akun-a"}}},
	}
}

// TestBelanjaBorosMenentukanWarna mengunci inti layar ini: warna dihitung dari
// UANG yang mengalir ke kampanye bermasalah, bukan dari jumlah kampanyenya.
func TestBelanjaBorosMenentukanWarna(t *testing.T) {
	w := layananUji(t, sumberUji{iklan: iklanUji()}).Susun(context.Background(), "tok", WRLingkup{})

	if w.Iklan.Belanja != 1_600_000 {
		t.Fatalf("belanja = %v, mau 1.600.000", w.Iklan.Belanja)
	}
	// Hanya c1 yang dihitung boros: c3 bermasalah tapi masih di bawah ambang
	// belajar, dan menghukumnya akan mendorong orang menghentikan kampanye baru
	// sebelum ia sempat belajar.
	if w.Iklan.BelanjaBoros != 1_000_000 {
		t.Fatalf("belanja boros = %v, mau 1.000.000 (c3 di bawah ambang belajar)", w.Iklan.BelanjaBoros)
	}
	if w.Iklan.PersenBelanjaBoros != 62.5 {
		t.Fatalf("persen boros = %v, mau 62,5", w.Iklan.PersenBelanjaBoros)
	}
	if w.StatusSistem.BelanjaIklan != WarnaMerah || w.StatusSistem.Umum != WarnaMerah {
		t.Fatalf("status = %+v, mau belanja iklan & umum merah", w.StatusSistem)
	}

	if len(w.KampanyeBoros) != 2 || w.KampanyeBoros[0].ID != "c1" {
		t.Fatalf("daftar boros tidak urut dari yang paling mahal: %+v", w.KampanyeBoros)
	}
	var c3 *WRKampanye
	for i := range w.KampanyeBoros {
		if w.KampanyeBoros[i].ID == "c3" {
			c3 = &w.KampanyeBoros[i]
		}
	}
	if c3 == nil || !c3.DiBawahAmbangBelajar {
		t.Fatal("kampanye baru harus ikut terlihat DAN ditandai di bawah ambang belajar")
	}
	if len(w.KampanyeTerbaik) != 1 || w.KampanyeTerbaik[0].ID != "c2" {
		t.Fatalf("kampanye terbaik = %+v, mau c2", w.KampanyeTerbaik)
	}
}

// TestLingkupProyekMemakaiRumusYangSama: menyaring satu proyek hanya mengurangi
// datanya — rumusnya persis sama, jadi angkanya harus cocok dengan penjumlahan
// kampanye proyek itu saja.
func TestLingkupProyekMemakaiRumusYangSama(t *testing.T) {
	svc := layananUji(t, sumberUji{iklan: iklanUji(), proyek: proyekUji()})
	w := svc.Susun(context.Background(), "tok", WRLingkup{ProyekID: "7"})

	if w.Lingkup.Nama != "GP Mawar" {
		t.Fatalf("nama lingkup = %q", w.Lingkup.Nama)
	}
	if w.Iklan.Belanja != 500_000 || w.Iklan.Hasil != 10 {
		t.Fatalf("belanja/hasil tersaring = %v/%v, mau 500.000/10", w.Iklan.Belanja, w.Iklan.Hasil)
	}
	if w.Iklan.BiayaPerHasil != 50000 {
		t.Fatalf("biaya per hasil = %v, mau 50.000", w.Iklan.BiayaPerHasil)
	}
	// Kampanye borosnya milik akun proyek lain, jadi lingkup ini bersih.
	if len(w.KampanyeBoros) != 0 || w.StatusSistem.BelanjaIklan != WarnaHijau {
		t.Fatalf("lingkup GP Mawar seharusnya bersih: %d boros, status %s", len(w.KampanyeBoros), w.StatusSistem.BelanjaIklan)
	}
	// Daftar proyek ikut menyempit — layar dan AI harus melihat cakupan sama.
	if len(w.Proyek) != 1 || w.Proyek[0].Nama != "GP Mawar" {
		t.Fatalf("daftar proyek = %+v, mau hanya GP Mawar", w.Proyek)
	}
	if len(w.ProyekTersedia) != 2 {
		t.Fatalf("pilihan lingkup harus tetap lengkap: %+v", w.ProyekTersedia)
	}
}

// TestBelanjaPerProyekDariPetaAkun: tanpa data Sales, perbandingan antar proyek
// bersandar pada peta proyek→akun iklan. Kalau pemetaan ini salah dibaca,
// usulan "alihkan budget dari A ke B" akan menunjuk proyek yang keliru.
func TestBelanjaPerProyekDariPetaAkun(t *testing.T) {
	s := sumberUji{
		iklan:  iklanUji(),
		proyek: proyekUji(),
	}
	w := layananUji(t, s).Susun(context.Background(), "tok", WRLingkup{})

	if len(w.Proyek) != 2 {
		t.Fatalf("proyek = %d, mau 2", len(w.Proyek))
	}
	// Urut dari belanja terbesar: akun-a (1,1 juta) sebelum akun-b (500rb).
	if w.Proyek[0].Nama != "GP Melati" || w.Proyek[0].Belanja != 1_100_000 || w.Proyek[0].Kampanye != 2 {
		t.Fatalf("proyek teratas = %+v, mau GP Melati 1.100.000 dari 2 kampanye", w.Proyek[0])
	}
	if w.Proyek[1].Nama != "GP Mawar" || w.Proyek[1].BiayaPerHasil != 50000 {
		t.Fatalf("proyek kedua = %+v, mau GP Mawar biaya per hasil 50.000", w.Proyek[1])
	}
	// Proyek yang tidak punya akun iklan sama sekali tetap muncul dengan nol —
	// hilang dari daftar akan membuatnya tak pernah ditanyakan.
	if w.Proyek[0].Kampanye+w.Proyek[1].Kampanye != 3 {
		t.Fatalf("seluruh kampanye harus terbagi ke proyeknya: %+v", w.Proyek)
	}
}

// TestProduksiKontenJadiDimensiKeempat: keadaan Alur Konten milik Marketing
// sendiri yang menggantikan dimensi hasil penjualan.
func TestProduksiKontenJadiDimensiKeempat(t *testing.T) {
	konten := func(context.Context) (KeadaanKonten, error) {
		return KeadaanKonten{
			Ringkas:   WRKonten{Sumber: SumberKonten, Pekerjaan: 6, LangkahTerbuka: 9, Terlambat: 3, SegeraJatuhTempo: 1},
			Tersendat: []WRKontenItem{{ID: "12", Judul: "Konten Reels Mawar", Langkah: "Produksi", Tingkat: WarnaMerah}},
			Orang:     []WROrang{{Peran: "Copywriter", Nama: "Rani", LangkahAktif: 4, Terlambat: 2, Selesai30Hari: 7}},
		}, nil
	}
	w := layananKonten(t, sumberUji{iklan: iklanUji()}, konten).Susun(context.Background(), "tok", WRLingkup{})
	if w.StatusSistem.ProduksiKonten != WarnaMerah {
		t.Fatalf("3 langkah terlambat seharusnya merah, dapat %q", w.StatusSistem.ProduksiKonten)
	}
	if w.Konten.Pekerjaan != 6 || len(w.KontenTersendat) != 1 {
		t.Fatalf("keadaan konten tidak terbawa: %+v / %+v", w.Konten, w.KontenTersendat)
	}
	// Performa orang ikut muatan yang sama — satu bacaan basis data untuk
	// seluruh dimensi konten.
	if len(w.Orang) != 1 || w.Orang[0].Nama != "Rani" || w.Orang[0].Selesai30Hari != 7 {
		t.Fatalf("performa orang tidak terbawa: %+v", w.Orang)
	}
}

// TestKanalMatiJadiAbuBukanHijau mengunci pembeda yang paling mudah salah:
// tidak ada data BUKAN berarti tidak ada masalah.
func TestKanalMatiJadiAbuBukanHijau(t *testing.T) {
	w := layananUji(t, sumberUji{iklan: iklanUji()}).Susun(context.Background(), "tok", WRLingkup{})
	if w.StatusSistem.ProduksiKonten != WarnaAbu {
		t.Fatalf("tanpa konten berjalan, produksi konten = %q, mau abu", w.StatusSistem.ProduksiKonten)
	}
	if len(w.CelahData) == 0 {
		t.Fatal("dimensi abu wajib menyebutkan alasannya di celah_data")
	}

	// Semua sumber mati → umum abu, bukan hijau.
	kosong := layananUji(t, sumberUji{}).Susun(context.Background(), "tok", WRLingkup{})
	if kosong.StatusSistem.Umum != WarnaAbu {
		t.Fatalf("tanpa data sama sekali, status umum = %q, mau abu", kosong.StatusSistem.Umum)
	}
}

// TestBatasBacaanSelaluDisebut: layar ini tidak membaca stok maupun akad, dan
// itu harus tertulis setiap kali — bukan cuma di dokumentasi. Tanpa kalimat itu
// orang akan mengira usulan budget di sini sudah menimbang ketersediaan unit.
func TestBatasBacaanSelaluDisebut(t *testing.T) {
	w := layananUji(t, sumberUji{iklan: iklanUji()}).Susun(context.Background(), "tok", WRLingkup{})
	ada := false
	for _, c := range w.CelahData {
		if c == celahTetap[0] {
			ada = true
		}
	}
	if !ada {
		t.Fatalf("batas bacaan (stok/booking/akad) tidak disebut: %+v", w.CelahData)
	}
}

// TestSumberMatiTidakMerobohkanLayar: satu layanan yang galat hanya membuat
// dimensinya abu; sisa angkanya tetap tampil.
func TestSumberMatiTidakMerobohkanLayar(t *testing.T) {
	konten := func(context.Context) (KeadaanKonten, error) {
		return KeadaanKonten{Ringkas: WRKonten{Sumber: SumberKonten, Pekerjaan: 3, LangkahTerbuka: 4}}, nil
	}
	s := sumberUji{errIklan: errors.New("sambungan ditolak")}
	w := layananKonten(t, s, konten).Susun(context.Background(), "tok", WRLingkup{})

	// Angka konten tetap tampil walau bacaan iklan gagal.
	if w.Konten.Pekerjaan != 3 || w.StatusSistem.ProduksiKonten != WarnaHijau {
		t.Fatalf("angka konten hilang gara-gara data iklan mati: %+v / %s", w.Konten, w.StatusSistem.ProduksiKonten)
	}
	if w.StatusSistem.BelanjaIklan != WarnaAbu {
		t.Fatalf("belanja iklan = %q, mau abu", w.StatusSistem.BelanjaIklan)
	}
	cocok := false
	for _, c := range w.CelahData {
		if c == "Data iklan tidak bisa dibaca: sambungan ditolak" {
			cocok = true
		}
	}
	if !cocok {
		t.Fatalf("alasan kegagalan tidak sampai ke celah_data: %+v", w.CelahData)
	}
}

// TestSinggahanMenahanPukulanBerulang: layar menyegarkan diri tiap menit dan
// tiap ada dorongan realtime. Tanpa singgahan, tiap penyegaran itu memukul Graph
// API Meta lagi — lambat bagi yang menunggu, dan boros kuota bagi semua orang.
func TestSinggahanMenahanPukulanBerulang(t *testing.T) {
	s := &sumberHitung{iklan: iklanUji()}
	svc := NewWarRoomService(s, nil, nil, WRAturan{BiayaPerHasilTarget: 100000}, "30d")
	jam := jamUji
	svc.SetJam(func() time.Time { return jam })

	for i := 0; i < 3; i++ {
		svc.Susun(context.Background(), "tok", WRLingkup{})
	}
	if s.panggil != 1 {
		t.Fatalf("sumber dipanggil %d kali dalam satu menit, mau 1", s.panggil)
	}
	// Lingkup lain punya singgahannya sendiri — angkanya memang beda.
	svc.Susun(context.Background(), "tok", WRLingkup{ProyekID: "7"})
	if s.panggil != 2 {
		t.Fatalf("lingkup baru harus dihitung ulang, panggilan = %d", s.panggil)
	}
	// Lewat umur singgahan, data dibaca lagi.
	jam = jam.Add(2 * time.Minute)
	svc.Susun(context.Background(), "tok", WRLingkup{})
	if s.panggil != 3 {
		t.Fatalf("setelah singgahan basi harus membaca ulang, panggilan = %d", s.panggil)
	}
}

// sumberHitung menghitung berapa kali datanya benar-benar dibaca.
type sumberHitung struct {
	iklan   IklanMentah
	panggil int
}

func (s *sumberHitung) Iklan(context.Context, string) (IklanMentah, error) {
	s.panggil++
	return s.iklan, nil
}
func (s *sumberHitung) IklanRinci(context.Context, string) (IklanRinciMentah, error) {
	return IklanRinciMentah{}, nil
}
func (s *sumberHitung) ProyekMeta(context.Context, string) ([]ProyekMeta, error) { return nil, nil }

// TestChipLingkupHanyaProyekBerakunMeta: peta proyek di metaapi memuat DUA jenis
// baris — proyek jualan, dan wadah tim pelaksana yang dinamai bebas seperti
// "Team SPV 1". Tanpa saringan, chip LINGKUP meminta orang di ruang rapat
// memilih "Team SPV 2" sebagai lingkup angka iklan — pilihan yang tidak berarti
// apa-apa.
//
// Penandanya AKUN META, bukan anggota sales: di data nyata keempat baris punya
// tepat satu orang sales, termasuk ketiga wadah tim, jadi sales tidak memisahkan
// apa pun.
func TestChipLingkupHanyaProyekBerakunMeta(t *testing.T) {
	pilih := pilihanProyek([]ProyekMeta{
		// Wadah tim: punya sales, tapi nol akun Meta. Persis bentuk data nyata.
		{ID: 9, Name: "Team SPV 1", Sales: []SalesProyekMeta{{Email: "s1@x.id"}}},
		{ID: 7, Name: "GP Mawar", Sales: []SalesProyekMeta{{Email: "a@x.id"}},
			Accounts: []AkunProyekMeta{{Kind: "ad", Ref: "akun-b"}}},
		{ID: 10, Name: "Team SPV 2", Sales: []SalesProyekMeta{{Email: "s2@x.id"}}},
		// WA/IG saja sudah cukup — lihat alasannya di pilihanProyek().
		{ID: 8, Name: "GP Melati", Accounts: []AkunProyekMeta{{Kind: "wa", Ref: "wa1"}}},
	})
	if len(pilih) != 2 {
		t.Fatalf("chip lingkup = %+v, mau hanya dua proyek berakun Meta", pilih)
	}
	for _, p := range pilih {
		if p.Nama == "Team SPV 1" || p.Nama == "Team SPV 2" {
			t.Fatalf("wadah tim pelaksana ikut jadi chip lingkup: %+v", pilih)
		}
	}
	// Urut nama, supaya posisi chip tidak berpindah-pindah antar pembacaan.
	if pilih[0].Nama != "GP Mawar" || pilih[1].Nama != "GP Melati" {
		t.Fatalf("urutan chip tidak stabil: %+v", pilih)
	}
}

// TestChipLingkupTidakPernahKosong menjaga jalur cadangannya. Akun Meta ditautkan
// belakangan lewat menu Akun Meta, jadi peta yang belum dilengkapi akan
// menyusutkan pemilih lingkup jadi satu tombol "Semua proyek" — kontrol yang
// rusak, dan yang melihatnya akan menyangka layarnya gagal memuat, bukan datanya
// yang belum ada.
func TestChipLingkupTidakPernahKosong(t *testing.T) {
	pilih := pilihanProyek([]ProyekMeta{
		{ID: 9, Name: "Team SPV 1"},
		{ID: 10, Name: "Team SPV 2"},
	})
	if len(pilih) != 2 {
		t.Fatalf("tanpa satu pun akun Meta, saringan harus dilepas: %+v", pilih)
	}
}

// TestHanyaKampanyeAktifYangDihitung mengunci permintaan pemilik repo: seluruh
// angka layar disusun dari kampanye yang MASIH BERJALAN.
//
// Layar rapat dipakai memutuskan apa yang harus dilakukan hari ini. Kampanye
// yang sudah dimatikan tidak bisa ditindaklanjuti, tapi belanjanya menaikkan
// total dan menyeret biaya-per-hasil — sehingga keputusan diambil dari angka
// yang sebagian miliknya kampanye yang sudah tidak ada.
func TestHanyaKampanyeAktifYangDihitung(t *testing.T) {
	m := IklanMentah{Configured: true}
	aktif := kampanye("a1", "Masih jalan", "akun-a", 1_000_000, 20, 2.0, 1.5)
	mati := kampanye("a2", "Sudah dimatikan", "akun-a", 4_000_000, 5, 0.5, 3)
	mati.EffectiveStatus = "CAMPAIGN_PAUSED"
	m.Campaigns = append(m.Campaigns, aktif, mati)

	svc := NewWarRoomService(nil, nil, nil, WRAturan{BiayaPerHasilTarget: 100000}, "30d")
	out, _, _ := svc.hitungIklan(m, saringan{})

	if out.Belanja != 1_000_000 {
		t.Fatalf("belanja = %v, mau 1jt (hanya yang aktif)", out.Belanja)
	}
	if out.Hasil != 20 {
		t.Fatalf("hasil = %v, mau 20 (hanya yang aktif)", out.Hasil)
	}
	if out.KampanyeAktif != 1 {
		t.Fatalf("kampanye aktif = %d, mau 1", out.KampanyeAktif)
	}
	// Yang dikecualikan TIDAK boleh hilang tanpa jejak: sesudah saringan ini
	// "Belanja" tidak lagi sama dengan laporan Meta untuk periode yang sama, dan
	// yang membandingkan dua layar harus bisa melihat selisihnya.
	if out.KampanyeNonAktif != 1 || out.BelanjaNonAktif != 4_000_000 {
		t.Fatalf("yang dikecualikan harus tetap dilaporkan: %d kampanye, belanja %v",
			out.KampanyeNonAktif, out.BelanjaNonAktif)
	}
}

// TestEffectiveStatusMengalahkanStatus: Meta memisahkan kedua kolom itu justru
// untuk kasus ini. Kampanye bisa ber-status ACTIVE sementara effectiveStatus-nya
// ADSET_PAUSED — ia tidak tayang, tapi kolom status miliknya sendiri tidak
// pernah berubah. Memakai `status` akan menghitungnya sebagai aktif selamanya.
func TestEffectiveStatusMengalahkanStatus(t *testing.T) {
	c := kampanye("x", "Adset dimatikan", "akun-a", 500_000, 3, 1.0, 1)
	c.Status = "ACTIVE"
	c.EffectiveStatus = "ADSET_PAUSED"
	if kampanyeAktif(c) {
		t.Fatal("status ACTIVE tidak boleh mengalahkan effectiveStatus ADSET_PAUSED")
	}
	// Muatan lama tidak mengirim effectiveStatus sama sekali; di situ `status`
	// yang dipakai, supaya layar tidak kosong tanpa sebab yang bisa dilihat.
	c.EffectiveStatus = ""
	if !kampanyeAktif(c) {
		t.Fatal("tanpa effectiveStatus, status ACTIVE harus dipakai sebagai cadangan")
	}
}
