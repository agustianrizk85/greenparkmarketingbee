package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

// HandlerFunc memproses satu event. Error hanya dicatat: reaksi backbone
// (segarkan dashboard, batalkan cache) idempoten — membiarkan jatuh lebih
// murah daripada mengantre ulang dan menabrak bump dua kali.
type HandlerFunc func(ctx context.Context, ev Event) error

// Consumer berlangganan topik-topik sebagai satu consumer group per SERVICE
// (group gp-<source>): tiap SERVICE menerima semua event sekali, dan reaksi
// antar-service saling tak bergantung.
//
// PERHATIAN pada kata "service" di atas — ia berarti service, BUKAN proses.
// Semua instance dari service yang sama memakai group id yang sama, jadi Kafka
// membagi partisi di antara mereka: tiap event hanya sampai ke SATU instance.
// Itu semantik pembagian beban, dan benar untuk pekerjaan yang cukup dikerjakan
// sekali. Ia SALAH untuk fan-out WebSocket, karena penonton yang tersambung ke
// instance lain tidak akan pernah diberi tahu. Untuk itu ada NewSiaranLokal.
type Consumer struct {
	r      *kafkago.Reader
	cfg    Config
	h      HandlerFunc
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	start  atomic.Bool
	// tanpaGroup: reader partisi langsung, tanpa consumer group. Offsetnya tidak
	// disimpan di broker, jadi CommitMessages justru menghasilkan galat.
	tanpaGroup bool
}

// NewConsumer menyiapkan langganan. Mengembalikan nil bila cfg.Disabled —
// semua metode aman pada nil. Topik dipastikan ada lebih dulu (ensureTopics);
// kalau broker belum bisa dihubungi saat start, consumer tetap jalan dan
// menunggu dengan mundur di log, bukan berhenti.
func NewConsumer(cfg Config, topics []string, h HandlerFunc) *Consumer {
	if cfg.Disabled || len(topics) == 0 || h == nil {
		return nil
	}
	if err := ensureTopics(cfg, AllTopics()); err != nil {
		log.Printf("kafka[%s]: pastikan topik: %v — consumer lanjut dan mencoba lagi saat broker siap", cfg.Source, err)
	}
	r := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:     cfg.Brokers,
		GroupID:     "gp-" + cfg.Source,
		GroupTopics: topics,
		MinBytes:    1,
		MaxBytes:    1 << 20,
		// Sinyal realtime tidak butuh sejarah: group baru mulai dari event
		// TERBARU, bukan memutar ulang seluruh masa lalu.
		StartOffset: kafkago.LastOffset,
	})
	log.Printf("kafka[%s]: consumer aktif (group gp-%s, topik: %s)", cfg.Source, cfg.Source, strings.Join(topics, ", "))
	ctx, cancel := context.WithCancel(context.Background())
	return &Consumer{r: r, cfg: cfg, h: h, ctx: ctx, cancel: cancel, done: make(chan struct{})}
}

// NewSiaranLokal menyiapkan saluran kedua untuk fan-out WebSocket: langganan
// topik divisi SENDIRI, TANPA consumer group. Mengapa perlu: tulisan yang
// ditangani instance A harus sampai ke penonton di instance B juga, tapi
// NewConsumer tidak pernah membawanya — OtherDataTopics memang mengecualikan
// topik sendiri, dan group gp-<source> yang dibagi semua instance membuat
// Kafka menyerahkan tiap peristiwa ke SATU instance saja. Reader partisi
// langsung tidak ikut pembagian itu: SETIAP instance menerima SETIAP
// peristiwa divisinya sendiri. Offset tidak tersimpan dan tidak ada rebalance
// — siaran memang tidak butuh keduanya. NewConsumer dibiarkan apa adanya
// karena pembagian beban antar-service memang tugasnya. Mengembalikan nil
// bila cfg.Disabled — semua metode aman pada nil.
func NewSiaranLokal(cfg Config, h HandlerFunc) *Consumer {
	if cfg.Disabled || h == nil {
		return nil
	}
	topic := DataTopic(cfg.Source)
	r := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:   cfg.Brokers,
		Topic:     topic,
		Partition: 0,
		MinBytes:  1,
		MaxBytes:  1 << 20,
	})
	// Reader tanpa group mengulang dari pesan paling awal secara bawaan;
	// siaran hanya peduli peristiwa SETELAH instance hidup, bukan sejarah.
	// (StartOffset di ReaderConfig cuma berlaku untuk group, makanya
	// offset awalnya ditetapkan lewat SetOffset di sini.)
	if err := r.SetOffset(kafkago.LastOffset); err != nil {
		log.Printf("kafka[%s]: set offset siaran lokal: %v", cfg.Source, err)
	}
	log.Printf("kafka[%s]: siaran lokal aktif (tanpa group, topik: %s)", cfg.Source, topic)
	ctx, cancel := context.WithCancel(context.Background())
	return &Consumer{r: r, cfg: cfg, h: h, ctx: ctx, cancel: cancel, done: make(chan struct{}), tanpaGroup: true}
}

// Start menjalankan loop baca-proses di goroutine sendiri. Idempoten: start
// kedua tidak melakukan apa-apa.
func (c *Consumer) Start() {
	if c == nil || !c.start.CompareAndSwap(false, true) {
		return
	}
	go c.run()
}

func (c *Consumer) run() {
	defer close(c.done)
	for {
		m, err := c.r.FetchMessage(c.ctx)
		if err != nil {
			if c.ctx.Err() != nil {
				return // shutdown
			}
			// Broker belum hidup / sambungan lepas — mundur, jangan meraung.
			log.Printf("kafka[%s]: baca gagal: %v", c.cfg.Source, err)
			select {
			case <-time.After(3 * time.Second):
			case <-c.ctx.Done():
				return
			}
			continue
		}
		var ev Event
		if err := json.Unmarshal(m.Value, &ev); err != nil {
			log.Printf("kafka[%s]: event tak terbaca (topik %s): %v", c.cfg.Source, m.Topic, err)
		} else {
			ev.Topic = m.Topic
			if err := c.h(c.ctx, ev); err != nil {
				log.Printf("kafka[%s]: tangani %s/%s: %v", c.cfg.Source, m.Topic, ev.Type, err)
			}
		}
		// Commit apa pun hasilnya: bump yang hilang lebih murah daripada
		// pesan busuk yang diantre ulang selamanya (poison-pill).
		//
		// Dilewati untuk reader tanpa group: offsetnya tidak disimpan di broker
		// sama sekali, dan memanggil commit di sana hanya menghasilkan galat
		// berulang di log tiap pesan.
		if !c.tanpaGroup {
			if err := c.r.CommitMessages(c.ctx, m); err != nil {
				log.Printf("kafka[%s]: commit: %v", c.cfg.Source, err)
			}
		}
	}
}

// Close menghentikan loop dan menutup reader. Aman pada nil.
func (c *Consumer) Close() {
	if c == nil {
		return
	}
	c.cancel()
	if c.start.Load() {
		<-c.done
	}
	if err := c.r.Close(); err != nil {
		log.Printf("kafka[%s]: consumer close: %v", c.cfg.Source, err)
	}
}

// ensureTopics membuat topik yang belum ada (1 partisi, RF 1 — skala satu
// broker). Idempoten: topik yang sudah ada dilewati.
func ensureTopics(cfg Config, topics []string) error {
	if len(topics) == 0 {
		return nil
	}
	conn, err := kafkago.Dial("tcp", cfg.Brokers[0])
	if err != nil {
		return err
	}
	defer conn.Close()
	for _, t := range topics {
		if err := conn.CreateTopics(kafkago.TopicConfig{Topic: t, NumPartitions: 1, ReplicationFactor: 1}); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "already exists") {
				return fmt.Errorf("topik %s: %w", t, err)
			}
		}
	}
	return nil
}

// bumpMinInterval adalah jendela peredam BumpOn: event lain divisi lain sering
// datang berjajaran (satu aksi menulis banyak rute), dan cukup satu dorongan
// refresh per ~250 ms.
const bumpMinInterval = 250 * time.Millisecond

// BumpOn membungkus bump menjadi HandlerFunc dengan peredam leading-edge:
// panggilan PERTAMA langsung dijalankan, sisanya dalam jendela 250 ms
// digabung. Penerima tetap segar tanpa ribuan broadcast WS saat rebahan.
func BumpOn(bump func()) HandlerFunc {
	var mu sync.Mutex
	var last time.Time
	return func(context.Context, Event) error {
		mu.Lock()
		fresh := time.Since(last) >= bumpMinInterval
		if fresh {
			last = time.Now()
		}
		mu.Unlock()
		if fresh {
			bump()
		}
		return nil
	}
}
