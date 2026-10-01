package handler

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

/*
 * MENDENGARKAN metaapi lewat Kafka, lalu mendorong layar.
 *
 * MASALAH YANG DIPECAHKAN. War Room Marketing (dan War Room di konsol CEO,
 * komponen yang sama) sudah punya soket ke service ini dan memuat ulang begitu
 * hub berdenyut — polling 60 detik cuma cadangan. Tapi hub ini hanya berdenyut
 * untuk tulisan service ini SENDIRI. Angka iklan datang dari metaapi, dan
 * perubahan di sana (kampanye ditandai proyeknya, akun Meta disambungkan,
 * tanda dilepas) tidak pernah sampai ke sini. Akibatnya layar baru berubah
 * pada polling berikutnya — sampai satu menit, dan di layar rapat jeda itu
 * terbaca sebagai "datanya tidak masuk".
 *
 * metaapi SUDAH memancarkannya. Setiap tulis sukses di sana lewat middleware
 * `EmitOnWriteGin` ke topik `gp.meta.data`. Yang hilang separuhnya: tidak ada
 * yang mendengarkan.
 *
 * KENAPA TIDAK MEMAKAI greenpark-shared/kafka. Seluruh backend di `backend/*`
 * memakai paket bersama itu lewat `replace => ../greenpark-shared`, yaitu
 * tetangga sefolder. Service ini tinggal di repo lain (`greenparkmarketingbee`),
 * dan jarak relatifnya ke paket itu BERBEDA antara laptop dan server:
 *
 *   laptop : greenparkjob/greenparkmarketingbee/backend  → ../../backend/greenpark-shared
 *   server : /opt/apps/greenparkmarketingbee/backend     → ../../greenpark-shared
 *
 * Satu `replace` tidak bisa benar di keduanya, dan yang salah tidak gagal saat
 * ditulis melainkan saat `go build` di server — di tengah deploy. Jadi
 * konsumennya berdiri sendiri di sini, dan satu-satunya yang ditiru dari paket
 * bersama adalah NAMA TOPIKNYA.
 *
 * Nama topik itu kontrak lintas service. Kalau `greenpark-shared/kafka/topics.go`
 * mengubah pola `gp.<divisi>.data`, baris di bawah ini ikut berubah — dan
 * perubahan itu tidak akan ketahuan dari kompilasi, hanya dari layar yang
 * berhenti hidup. Komentar ini ada supaya `grep gp.meta.data` menemukan keduanya.
 */

// topikMeta = `kafka.DataTopic(kafka.SourceMeta)` di greenpark-shared.
const topikMeta = "gp.meta.data"

// grupKonsumen dipisah dari milik divisi lain supaya service ini membaca
// seluruh pesan sejak ia menyala, bukan berebut partisi dengan konsumen lain.
const grupKonsumen = "gp-marketingflow-warroom"

// KonsumenMeta membaca event metaapi dan memanggil `bump` untuk tiap pesan.
type KonsumenMeta struct {
	r    *kafka.Reader
	bump func()
	stop context.CancelFunc
}

/*
 * MulaiKonsumenMeta menyalakan konsumen bila KAFKA_BROKERS terisi.
 *
 * Kosong atau "off" = MATI, dan itu bukan kegagalan: laptop tanpa Docker dan
 * server yang belum menaikkan broker harus tetap bisa menjalankan service ini.
 * Yang hilang cuma kesegeraan — polling 60 detik di layar tetap jalan, jadi
 * angkanya tetap benar, hanya terlambat. Mematikan service karena broker tidak
 * ada akan menukar keterlambatan satu menit dengan layar yang mati total.
 *
 * `bump` dipanggil dari goroutine konsumen; hub sudah aman dipakai bersamaan.
 */
func MulaiKonsumenMeta(bump func()) *KonsumenMeta {
	brokers := pecahBrokers(os.Getenv("KAFKA_BROKERS"))
	if len(brokers) == 0 {
		log.Printf("kafka: KAFKA_BROKERS kosong/off — War Room mengandalkan polling 60 dtk")
		return nil
	}
	ctx, batal := context.WithCancel(context.Background())
	k := &KonsumenMeta{
		r: kafka.NewReader(kafka.ReaderConfig{
			Brokers: brokers,
			Topic:   topikMeta,
			GroupID: grupKonsumen,
			// Mulai dari pesan TERBARU, bukan dari awal. Riwayat perubahan metaapi
			// tidak ada gunanya di sini: yang dibutuhkan cuma "ada yang berubah,
			// baca ulang". Membaca dari awal akan membunyikan hub ratusan kali saat
			// service menyala dan membuat tiap layar memuat ulang beruntun.
			StartOffset: kafka.LastOffset,
			MaxWait:     time.Second,
		}),
		bump: bump,
		stop: batal,
	}
	go k.jalan(ctx)
	log.Printf("kafka: mendengarkan %s di %v (grup %s)", topikMeta, brokers, grupKonsumen)
	return k
}

func (k *KonsumenMeta) jalan(ctx context.Context) {
	for {
		_, err := k.r.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // ditutup, bukan galat
			}
			// Broker mati/putus: dicatat lalu dicoba lagi. Reader milik kafka-go
			// sudah menyambung ulang sendiri; jeda di sini hanya menahan banjir log
			// kalau brokernya memang tidak ada.
			log.Printf("kafka: baca %s gagal: %v", topikMeta, err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}
		// ISI pesan sengaja diabaikan. Yang dibutuhkan layar cuma "ada yang
		// berubah di metaapi, baca ulang" — dan membaca ulang lewat HTTP
		// menghasilkan muatan yang sudah dibersihkan dan disaring lingkupnya,
		// yang tidak mungkin dirakit dari satu pesan event.
		k.bump()
	}
}

// Tutup menghentikan konsumen. Aman dipanggil pada nil (konsumen tidak menyala).
func (k *KonsumenMeta) Tutup() {
	if k == nil {
		return
	}
	k.stop()
	_ = k.r.Close()
}

// pecahBrokers membaca daftar broker dipisah koma. "off" mematikan backbone —
// ejaan yang sama dengan greenpark-shared/kafka supaya satu env mematikan
// seluruh stack, bukan sebagian.
func pecahBrokers(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "off") {
		return nil
	}
	out := []string{}
	for _, b := range strings.Split(s, ",") {
		if b = strings.TrimSpace(b); b != "" {
			out = append(out, b)
		}
	}
	return out
}
