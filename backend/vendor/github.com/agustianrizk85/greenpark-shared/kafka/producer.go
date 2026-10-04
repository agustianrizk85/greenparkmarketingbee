package kafka

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

// Producer memancarkan event. Dibuat lewat NewProducer; NIL bila backbone
// dimatikan — dan SEMUA metodenya aman dipanggil pada nil (no-op), jadi
// pemanggil tidak perlu bercabang if di mana-mana.
type Producer struct {
	w      *kafkago.Writer
	source string
	topic  string // topik data generik divisi ini (gp.<source>.data)
}

// NewProducer menyiapkan pemancar async: WriteMessages kembali seketika,
// kiriman disatukan latar belakang — jalur tulis HTTP TIDAK PERNAH menunggu
// broker. Mengembalikan nil bila cfg.Disabled.
func NewProducer(cfg Config) *Producer {
	if cfg.Disabled || len(cfg.Brokers) == 0 {
		return nil
	}
	w := &kafkago.Writer{
		Addr:     kafkago.TCP(cfg.Brokers...),
		Balancer: &kafkago.Hash{}, // key sama → partisi sama → urut per resource
		AllowAutoTopicCreation: true,
		Async:        true,
		BatchTimeout: 10 * time.Millisecond, // latensi kirim ~belasan ms, bukan 0.5 dtk
		RequiredAcks: kafkago.RequireOne,
	}
	log.Printf("kafka[%s]: producer aktif → %s", cfg.Source, strings.Join(cfg.Brokers, ","))
	return &Producer{w: w, source: cfg.Source, topic: DataTopic(cfg.Source)}
}

// Enabled melaporkan apakah backbone hidup untuk service ini.
func (p *Producer) Enabled() bool { return p != nil }

// Emit memancarkan satu event. Fire-and-forget: kegagalan hanya masuk log —
// tulisan divisi tidak boleh gagal karena backbone realtime sedang mati.
func (p *Producer) Emit(ctx context.Context, topic, typ, key string, payload any) {
	if p == nil {
		return
	}
	body, err := json.Marshal(Event{
		ID:         newEventID(),
		Type:       typ,
		Source:     p.source,
		Key:        key,
		OccurredAt: time.Now(),
		Payload:    payload,
	})
	if err != nil {
		log.Printf("kafka[%s]: susun event %s/%s: %v", p.source, topic, typ, err)
		return
	}
	if err := p.w.WriteMessages(ctx, kafkago.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: body,
	}); err != nil {
		log.Printf("kafka[%s]: emit %s/%s gagal: %v", p.source, topic, typ, err)
	}
}

// EmitChanged memancarkan sinyal bawaan "data.changed" ke topik data divisi
// ini — dipakai middleware EmitOnWrite dan titik tulis manual di service.
func (p *Producer) EmitChanged(key string, payload any) {
	if p == nil {
		return
	}
	p.Emit(context.Background(), p.topic, TypeDataChanged, key, payload)
}

// Close mem-bus kiriman async yang belum keluar lalu menutup sambungan.
// Aman pada nil. Dipanggil via defer di composition root.
func (p *Producer) Close() {
	if p == nil {
		return
	}
	if err := p.w.Close(); err != nil {
		log.Printf("kafka[%s]: producer close: %v", p.source, err)
	}
}