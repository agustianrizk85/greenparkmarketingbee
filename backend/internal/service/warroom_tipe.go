package service

// BENTUK DATA WAR ROOM MARKETING.
//
// Satu berkas khusus untuk tipe supaya perhitungannya (warroom.go) dan
// pengambilan datanya (warroom_sumber.go) bisa dibaca tanpa terhalang deklarasi
// panjang.
//
// ATURAN YANG DIJAGA DI SELURUH PAKET INI: layar war room dan AI membaca ANGKA
// YANG SAMA, yaitu angka di struktur ini. AI tidak pernah menghitung; ia hanya
// menyusun kalimat dan usulan di atasnya. Karena itu setiap angka yang dipakai
// mengambil keputusan wajib punya tempatnya sendiri di sini — termasuk angka
// yang "hanya" jadi bahan usulan, seperti belanja kampanye yang dipertaruhkan.
//
// FIELD "sumber" ADA DI HAMPIR SEMUA BLOK dan itu disengaja: tiga sumber data
// war room ini punya kekuatan yang berbeda jauh. "meta" datang langsung dari
// akun iklan, "konten" dari basis data Marketing sendiri, "manual" dari
// ambang yang diketik orang. Keputusan besar yang bertumpu pada angka manual
// harus bisa dikenali sebagai keputusan yang lebih lemah — tanpa penanda ini,
// ketiganya terlihat sama meyakinkan di layar.

// Warna status satu dimensi. Sama persis dengan war room divisi lain supaya
// satu layar direksi tidak memakai dua kamus warna.
const (
	WarnaHijau  = "hijau"
	WarnaKuning = "kuning"
	WarnaMerah  = "merah"
	WarnaAbu    = "abu" // datanya tidak ada — BUKAN "baik-baik saja"
)

// Sumber angka.
const (
	SumberMeta   = "meta"
	SumberKonten = "konten"
	SumberManual = "manual"
)

// WarRoom adalah seluruh muatan GET /api/warroom.
type WarRoom struct {
	Divisi          string             `json:"divisi"`
	TanggalData     string             `json:"tanggal_data"` // RFC3339
	Lingkup         WRLingkupMeta      `json:"lingkup"`
	ProyekTersedia  []WRPilihan        `json:"proyek_tersedia"`
	Aturan          WRAturan           `json:"aturan"`
	StatusSistem    WRStatus           `json:"status_sistem"`
	Iklan           WRIklan            `json:"iklan"`
	KampanyeBoros   []WRKampanye       `json:"kampanye_boros"`
	KampanyeTerbaik []WRKampanye       `json:"kampanye_terbaik"`
	KreatifLelah    []WRKreatif        `json:"kreatif_lelah"`
	Proyek          []WRProyek         `json:"proyek"`
	Konten          WRKonten           `json:"konten"`
	KontenTersendat []WRKontenItem     `json:"konten_tersendat"`
	Orang           []WROrang          `json:"orang"`
	Corong          []WRTahapCorong    `json:"corong"`
	Tren            []WRTitikTren      `json:"tren"`
	Keputusan       WRKeputusanRingkas `json:"keputusan"`
	CelahData       []string           `json:"celah_data"`
}

// WRLingkup adalah penyaring yang diminta pemanggil.
type WRLingkup struct {
	ProyekID string
}

// WRLingkupMeta adalah lingkup yang BERLAKU, sudah dengan namanya — layar
// memakai ini untuk judul, dan AI memakainya untuk menyebut cakupan briefing.
type WRLingkupMeta struct {
	ProyekID string `json:"proyek_id"`
	Nama     string `json:"nama"`
}

// WRPilihan adalah satu pilihan lingkup.
type WRPilihan struct {
	ID   string `json:"id"`
	Nama string `json:"nama"`
}

// WRAturan adalah ambang yang dipakai menilai warna. Ikut dikirim ke AI, bukan
// disembunyikan di kode: usulan "biaya per hasil terlalu mahal" tidak ada artinya
// kalau pembacanya tidak tahu mahal dibanding apa.
type WRAturan struct {
	BiayaPerHasilTarget  float64  `json:"biaya_per_hasil_target"`
	AmbangBelajarBelanja float64  `json:"ambang_belajar_belanja"`
	FrekuensiMaks        float64  `json:"frekuensi_maks"`
	CTRMinPersen         float64  `json:"ctr_min_persen"`
	JamBalasLeadMaks     float64  `json:"jam_balas_lead_maks"`
	Sumber               string   `json:"sumber"`
	Penjelasan           []string `json:"penjelasan"`
}

// WRStatus adalah warna tiap dimensi. Umum = yang terburuk di antaranya.
type WRStatus struct {
	Umum           string `json:"umum"`
	BelanjaIklan   string `json:"belanja_iklan"`
	MutuLead       string `json:"mutu_lead"`
	ProduksiKonten string `json:"produksi_konten"`
}

// WRIklan adalah ringkasan belanja iklan pada rentang yang dibaca.
type WRIklan struct {
	Sumber        string  `json:"sumber"`
	Terpasang     bool    `json:"terpasang"`
	Rentang       string  `json:"rentang"`
	Belanja       float64 `json:"belanja"`
	Hasil         float64 `json:"hasil"`
	BiayaPerHasil float64 `json:"biaya_per_hasil"`
	Impresi       float64 `json:"impresi"`
	Klik          float64 `json:"klik"`
	CTR           float64 `json:"ctr"`
	CPC           float64 `json:"cpc"`
	// Jangkauan dijumlah antar kampanye, jadi satu orang yang melihat dua
	// kampanye terhitung dua kali; namanya menyebut itu supaya tidak dibaca
	// sebagai jumlah orang. Frekuensi TIDAK dirata-ratakan melainkan diambil yang
	// tertinggi: yang menentukan kapan sebuah kreatif harus diganti adalah
	// kampanye terparah, bukan rata-rata yang menyamarkannya.
	JangkauanTerjumlah float64 `json:"jangkauan_terjumlah"`
	FrekuensiTertinggi float64 `json:"frekuensi_tertinggi"`
	KampanyeAktif      int     `json:"kampanye_aktif"`
	KampanyeBermasalah int     `json:"kampanye_bermasalah"`
	BelanjaBoros       float64 `json:"belanja_boros"`
	PersenBelanjaBoros float64 `json:"persen_belanja_boros"`
}

// WRKampanye adalah satu kampanye beserta alasan ia masuk daftar.
type WRKampanye struct {
	ID            string  `json:"id"`
	Nama          string  `json:"nama"`
	Akun          string  `json:"akun"`
	Status        string  `json:"status"`
	Belanja       float64 `json:"belanja"`
	Hasil         float64 `json:"hasil"`
	BiayaPerHasil float64 `json:"biaya_per_hasil"`
	CTR           float64 `json:"ctr"`
	Frekuensi     float64 `json:"frekuensi"`
	Masalah       string  `json:"masalah"`
	// DiBawahAmbangBelajar = belanjanya belum cukup untuk dinilai. Dikirim
	// eksplisit supaya AI tidak perlu menghitung sendiri untuk menuruti larangan
	// "jangan hentikan kampanye di bawah ambang belajar".
	DiBawahAmbangBelajar bool   `json:"di_bawah_ambang_belajar"`
	Sumber               string `json:"sumber"`
}

// WRKreatif adalah satu naskah iklan yang lelah (sering dilihat, hasil turun).
type WRKreatif struct {
	ID            string  `json:"id"`
	Judul         string  `json:"judul"`
	Belanja       float64 `json:"belanja"`
	Hasil         float64 `json:"hasil"`
	BiayaPerHasil float64 `json:"biaya_per_hasil"`
	CTR           float64 `json:"ctr"`
	Impresi       float64 `json:"impresi"`
	Iklan         int     `json:"iklan"`
	Masalah       string  `json:"masalah"`
	Sumber        string  `json:"sumber"`
}

// WRKonten adalah keadaan produksi konten — pekerjaan Marketing sendiri, dibaca
// dari basis datanya sendiri (menu Alur Konten), bukan dari divisi lain.
//
// Dimensi ini menjawab yang tidak bisa dijawab angka iklan: biaya per hasil
// boleh murah, tapi kalau bahan kontennya macet seminggu di satu orang, belanja
// bulan depan berjalan tanpa materi baru.
type WRKonten struct {
	Sumber           string `json:"sumber"`
	Pekerjaan        int    `json:"pekerjaan"`
	LangkahTerbuka   int    `json:"langkah_terbuka"`
	Terlambat        int    `json:"terlambat"`
	SegeraJatuhTempo int    `json:"segera_jatuh_tempo"`
	ButuhMasukan     int    `json:"butuh_masukan"`
}

// WRKontenItem adalah satu langkah konten yang tersendat.
type WRKontenItem struct {
	ID      string `json:"id"`
	Judul   string `json:"judul"`
	Langkah string `json:"langkah"`
	Pemilik string `json:"pemilik"`
	Tingkat string `json:"tingkat"` // merah | kuning
	Pesan   string `json:"pesan"`
	Tenggat string `json:"tenggat"`
	Sumber  string `json:"sumber"`
}

// WROrang adalah beban DAN hasil kerja satu peran PIC di alur konten.
//
// Keduanya dibawa bersama dengan sengaja: menilai orang dari sisa pekerjaannya
// saja membuat yang paling produktif terlihat paling buruk — ia kebagian paling
// banyak justru karena cepat menyelesaikan.
type WROrang struct {
	Peran            string  `json:"peran"`
	Nama             string  `json:"nama"`
	LangkahAktif     int     `json:"langkah_aktif"`
	Terlambat        int     `json:"terlambat"`
	SegeraJatuhTempo int     `json:"segera_jatuh_tempo"`
	Selesai30Hari    int     `json:"selesai_30_hari"`
	TerakhirSelesai  *string `json:"terakhir_selesai"`
	Sumber           string  `json:"sumber"`
}

// WRProyek adalah satu proyek beserta belanja iklan dan lead-nya.
//
// Angkanya SELURUHNYA dari akun iklan dan kotak masuk milik proyek itu (peta
// proyek→akun di menu Akun Meta). Stok unit dan hasil penjualan TIDAK ada di
// sini: keduanya milik Sales, dan war room ini sengaja hanya membaca data yang
// memang ada di menu Marketing.
type WRProyek struct {
	ID            string  `json:"id"`
	Nama          string  `json:"nama"`
	Belanja       float64 `json:"belanja"`
	Hasil         float64 `json:"hasil"`
	BiayaPerHasil float64 `json:"biaya_per_hasil"`
	Kampanye      int     `json:"kampanye"`
	Sumber        string  `json:"sumber"`
}

// WRTahapCorong adalah satu tahap perjalanan uang: budget → … → akad.
type WRTahapCorong struct {
	Tahap          string  `json:"tahap"`
	Jumlah         float64 `json:"jumlah"`
	Satuan         string  `json:"satuan"`
	BiayaPerSatuan float64 `json:"biaya_per_satuan"`
	KonversiPersen float64 `json:"konversi_persen"`
	Sumber         string  `json:"sumber"`
}

// WRTitikTren adalah satu hari belanja iklan.
type WRTitikTren struct {
	Tanggal string  `json:"tanggal"`
	Belanja float64 `json:"belanja"`
	Hasil   float64 `json:"hasil"`
	Klik    float64 `json:"klik"`
	Impresi float64 `json:"impresi"`
}

// WRKeputusanRingkas adalah hitungan perintah yang tercatat di war room. Yang
// "telat" ikut memicu briefing baru, jadi ia dihitung sistem — bukan ditaksir AI.
type WRKeputusanRingkas struct {
	Terbuka      int `json:"terbuka"`
	Telat        int `json:"telat"`
	Selesai7Hari int `json:"selesai_7_hari"`
}

/* ---- bentuk MENTAH dari sumber luar ---------------------------------------
 *
 * Sengaja dipisah dari bentuk payload di atas. Kalau metaapi atau Sales mengubah
 * bentuk jawabannya, yang berubah hanya berkas sumber dan struktur di bawah ini;
 * perhitungan warna dan seluruh kontrak dengan layar serta AI tidak ikut goyah.
 *
 * Tiap baris daftar punya TIPE BERNAMA, bukan struct anonim di dalam induknya.
 * Selain lebih enak dibaca, itu yang membuat uji bisa merakit satu kampanye atau
 * satu proyek tanpa menyalin ulang seluruh deklarasi field-nya — dan uji yang
 * menyakitkan untuk ditulis adalah uji yang tidak ditulis.
 */

// KampanyeMentah = satu kampanye di jawaban metaapi.
type KampanyeMentah struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Account         string  `json:"account"`
	AccountID       string  `json:"accountId"`
	Status          string  `json:"status"`
	Spend           float64 `json:"spend"`
	Impressions     float64 `json:"impressions"`
	Clicks          float64 `json:"clicks"`
	Reach           float64 `json:"reach"`
	Frequency       float64 `json:"frequency"`
	CTR             float64 `json:"ctr"`
	CPC             float64 `json:"cpc"`
	Results         float64 `json:"results"`
	CostPerResult   float64 `json:"costPerResult"`
	EffectiveStatus string  `json:"effectiveStatus"`
	Issues          int     `json:"issues"`
	IssueSummary    string  `json:"issueSummary"`
}

// IklanMentah = /api/meta/ads milik metaapi.
type IklanMentah struct {
	Configured bool             `json:"configured"`
	Campaigns  []KampanyeMentah `json:"campaigns"`
	Error      string           `json:"error"`
}

// HariIklanMentah = satu hari belanja iklan.
type HariIklanMentah struct {
	Date        string  `json:"date"`
	Spend       float64 `json:"spend"`
	Results     float64 `json:"results"`
	Clicks      float64 `json:"clicks"`
	Impressions float64 `json:"impressions"`
}

// KreatifMentah = satu naskah iklan beserta hasilnya.
type KreatifMentah struct {
	Body          string  `json:"body"`
	Title         string  `json:"title"`
	Spend         float64 `json:"spend"`
	Results       float64 `json:"results"`
	CostPerResult float64 `json:"costPerResult"`
	Impressions   float64 `json:"impressions"`
	Clicks        float64 `json:"clicks"`
	CTR           float64 `json:"ctr"`
	Ads           int     `json:"ads"`
}

// IklanRinciMentah = /api/meta/ads/detail (tren harian + naskah iklan).
type IklanRinciMentah struct {
	Configured bool              `json:"configured"`
	Daily      []HariIklanMentah `json:"daily"`
	Creatives  []KreatifMentah   `json:"creatives"`
	Error      string            `json:"error"`
}

// AkunProyekMeta = satu akun (iklan/WhatsApp/Instagram) milik sebuah proyek.
type AkunProyekMeta struct {
	Kind  string `json:"kind"` // "wa" | "ig" | "ad"
	Ref   string `json:"ref"`
	Label string `json:"label"`
}

// ProyekMeta = /api/meta/projects — peta proyek ke akun iklan/WA/IG dan timnya.
type ProyekMeta struct {
	ID       int              `json:"id"`
	Name     string           `json:"name"`
	Accounts []AkunProyekMeta `json:"accounts"`
}
