// Package kafka adalah backbone event lintas divisi Greenpark.
//
// Setiap backend divisi memancarkan event "data.changed" ke topiknya sendiri
// (gp.<divisi>.data) setiap kali ada tulisan sukses, dan berlangganan topik
// divisi lain. Reaksi bawaan penerima cuma satu: menaikkan revisi realtime
// (pola bump-rev wsrev) sehingga setiap dashboard yang terhubung menarik ulang
// datanya — kini TERMASUK data lintas divisi yang dibaca lewat /api/xdiv/*.
// Sebelum backbone ini, tulisan di Permit cuma terlihat di dashboard divisi
// lain setelah polling / refresh manual; sekarang terlihat < 1 detik.
//
// Topik bertema (gp.legal.units, gp.meta.wa, …) membawa payload domain yang
// lebih kaya untuk reaksi spesifik — mis. Keuangan membatalkan cache peta
// proyek→GP-nya begitu Master Kavling berubah.
//
// Pemakaian di sebuah service (composition root / main.go):
//
//	prod := kafka.NewProducer(kafka.AutoConfig("sales")) // nil bila dimatikan
//	defer prod.Close()
//	handler.SetEvents(prod)
//	cons := kafka.NewConsumer(kafka.AutoConfig("sales"),
//		kafka.OtherDataTopics("sales"),
//		kafka.BumpOn(handler.Bump))
//	defer cons.Close()
//	if cons != nil {
//		cons.Start()
//	}
//	// di router.go: chain(mux, logger, kafka.EmitOnWrite(h.events), …)
//
// Kafka OPSIONAL: tanpa KAFKA_BROKERS yang valid (atau "off"), producer dan
// consumer menjadi no-op — semua service tetap berjalan seperti sebelumnya,
// dan prod tanpa broker tidak pernah menjatuhkan tulisan HTTP.
package kafka

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// Config adalah konfigurasi backbone untuk satu service.
type Config struct {
	Source  string   // kode divisi, mis. "sales", "finance" — jadi id group & topik
	Brokers []string // alamat broker, mis. ["localhost:9092"]
	Disabled bool   // true = service ini tidak ikut backbone (no-op)
}

// AutoConfig membaca KAFKA_BROKERS dari lingkungan:
//
//	kosong            → "localhost:9092" (bawaan dev; docker-compose menyediakannya)
//	"off"/"disabled"  → backbone dimatikan untuk service ini
//	"host:9092,…"     → daftar broker produksi (koma)
//
// Satu nama env untuk SEMUA service — backbone adalah kebijakan satu stack,
// bukan preferensi per divisi.
func AutoConfig(source string) Config {
	cfg := Config{Source: source, Brokers: []string{"localhost:9092"}}
	v := strings.TrimSpace(os.Getenv("KAFKA_BROKERS"))
	switch strings.ToLower(v) {
	case "":
		// kosong → bawaan dev
	case "off", "disabled", "none", "-":
		cfg.Disabled = true
		cfg.Brokers = nil
	default:
		cfg.Brokers = strings.Split(v, ",")
	}
	return cfg
}

// Event adalah amplop semua pesan backbone. Kontraknya hanya bidang amplop;
// isi payload milik divisi pemancar — penerima yang tidak mengenali type-nya
// cukup mengabaikannya (reaksi bawaan tetap jalan).
type Event struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Source     string    `json:"source"`
	Topic      string    `json:"-"` // diisi consumer dari topik Kafka (bukan bagian payload)
	Key        string    `json:"key,omitempty"`
	OccurredAt time.Time `json:"occurredAt"`
	Payload    any       `json:"payload,omitempty"`
}

// newEventID membuat identitas unik yang urut waktu: 12 hex milidetik + 10 hex
// acak — unik, mudah dibaca di log, dan ID lama selalu < ID baru.
func newEventID() string {
	var b [5]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%012x%010x", time.Now().UnixMilli(), b)
}

// isReadMethod melaporkan apakah method ini tulis (bukan baca/preflight).
func isReadMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}