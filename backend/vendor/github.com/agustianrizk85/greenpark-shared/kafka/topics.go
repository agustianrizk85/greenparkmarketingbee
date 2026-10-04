package kafka

// Kode divisi di backbone. Satu kode = satu service; kode inilah yang dipakai
// sebagai nama topik (gp.<kode>.data) dan suffix consumer group (gp-<kode>).
const (
	SourceAuth        = "auth"        // :8090 master auth + master data + papan
	SourceLegal       = "legal"       // :8081 legalpermit — hub master properti
	SourceFinance     = "finance"     // :8083 keungan
	SourceSales       = "sales"       // :8084
	SourceMarketing   = "marketing"   // :8085
	SourceMeta        = "meta"        // :8088 metaapi — WhatsApp/Instagram inbox
	SourcePerencanaan = "perencanaan" // :8082
	SourceTeknik      = "teknik"      // :8086
	SourceSDM         = "sdm"        // :8087
)

// Divisions adalah seluruh divisi yang ikut backbone. OtherDataTopics dan
// AllTopics membaca daftar ini — menambah divisi di sini otomatis menambah
// langganan semua service lain.
var Divisions = []string{
	SourceAuth, SourceLegal, SourceFinance, SourceSales, SourceMarketing,
	SourceMeta, SourcePerencanaan, SourceTeknik, SourceSDM,
}

// Tipe event generik: "sesuatu di divisi ini berubah, tarik ulang datamu".
const TypeDataChanged = "data.changed"

// Topik bertema — payload domain yang lebih kaya untuk reaksi spesifik.
// Berbeda dari gp.<divisi>.data (sinyal generik), topik ini dipancarkan dari
// titik tulis domain dan dibaca oleh consumer yang peduli pada domain itu.
const (
	TopicAuthUsers         = "gp.auth.users"           // user dibuat/diubah/dinonaktifkan (auth)
	TopicPerencanaanUnits  = "gp.perencanaan.units"    // master properti: tipe/blok/unit berubah
	TopicFinancePayments   = "gp.finance.payments"     // penerimaan tercatat (keuangan)
	TopicMetaWA            = "gp.meta.wa"              // pesan WA masuk/balasan terkirim (metaapi)
)

// DataTopic membangun nama topik data generik sebuah divisi.
func DataTopic(source string) string { return "gp." + source + ".data" }

// OtherDataTopics memilih semua topik data generik KECUALI divisi sendiri —
// langganan bawaan tiap consumer: "segarkan saya saat divisi lain menulis".
func OtherDataTopics(self string) []string {
	out := make([]string, 0, len(Divisions)-1)
	for _, d := range Divisions {
		if d != self {
			out = append(out, DataTopic(d))
		}
	}
	return out
}

// AllTopics memilih seluruh topik backbone — dipakai ensureTopics supaya topik
// sudah ada sebelum consumer mana pun menyapa broker.
func AllTopics() []string {
	out := make([]string, 0, len(Divisions)+5)
	for _, d := range Divisions {
		out = append(out, DataTopic(d))
	}
	return append(out,
		TopicAuthUsers, TopicPerencanaanUnits, TopicFinancePayments, TopicMetaWA)
}