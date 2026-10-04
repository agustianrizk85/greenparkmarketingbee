package handler

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"marketingflow/internal/authmw"
	"marketingflow/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// RealtimeHub keeps the set of connected dashboard browsers and pushes a
// data-revision message whenever the backend data changes — giving instant,
// no-refresh updates over a WebSocket. The browser re-fetches on each push.
//
// The revision is bumped by BumpMiddleware after every successful mutating
// request, so any write — by any user — fans out to every open dashboard.
type RealtimeHub struct {
	rev int64
	// mu melindungi conns SAJA — bukan penulisan ke jaringan. Dulu kunci yang
	// sama ditahan selama seluruh loop WriteJSON, sehingga satu klien lambat
	// menahannya sampai tenggat 5 detik dan ikut memblokir pendaftaran koneksi
	// BARU (add memakai mutex yang sama).
	mu sync.Mutex
	// tulis menjamin hanya SATU penulis pada satu waktu — syarat gorilla.
	tulis sync.Mutex
	// sinyal berkapasitas SATU; penyiar tunggal membaca dari sini (mulaiPenyiar).
	sinyal chan struct{}
	conns  map[*websocket.Conn]bool

	// muatan menghasilkan isi data yang ikut disiarkan. nil = hub PEMICU, yang
	// hanya mengirim {"rev": n}. Itu beda pokok antara kedua hub: /api/ws
	// berizin longgar KARENA muatannya nil, /api/ws/data berizin ketat KARENA
	// muatannya terisi. Jangan pernah mengisi muatan hub pemicu.
	muatan func() any

	// sumber = nama divisi, ikut di pesan sebagai "divisi". Layar Empat Divisi
	// berlangganan beberapa soket data sekaligus dengan bentuk muatan yang
	// berbeda-beda; penanda ini membuatnya tidak perlu menebak dari bentuk.
	sumber string

	// pengikut = hub data yang ikut disiarkan tiap kali hub ini di-bump, dengan
	// nomor revisi yang SAMA. Dipasang lewat Ikut(), sehingga BumpMiddleware dan
	// Bump() tidak perlu tahu ada hub lain.
	//
	// Irisan, bukan satu: satu rute /ws/data melayani DUA muatan — ringkas
	// untuk kartu Empat Divisi dan penuh untuk War Room Marketing — dan
	// keduanya harus disiarkan oleh bump yang sama. Dengan satu slot, hub kedua
	// yang dipasang akan diam-diam menggantikan yang pertama, dan salah satu
	// layar berhenti diperbarui tanpa satu galat pun.
	pengikut []*RealtimeHub
}

// WebSocket keepalive timings. The server pings periodically and expects a pong
// within pongWait; a missed pong trips the read deadline and reaps the (possibly
// half-open) connection, so goroutines don't linger and idle proxies don't drop
// the socket silently.
const (
	wsWriteWait  = 5 * time.Second
	wsPongWait   = 60 * time.Second
	wsPingPeriod = (wsPongWait * 9) / 10
)

func NewRealtimeHub() *RealtimeHub {
	h := &RealtimeHub{conns: map[*websocket.Conn]bool{}}
	h.mulaiPenyiar()
	return h
}

// mulaiPenyiar menyalakan SATU goroutine penyiar untuk hub ini; picu() jadi
// satu-satunya cara memintanya menyiarkan.
//
// Menggantikan pola "go broadcast(rev)" per tulisan, yang punya dua cacat:
// URUTAN (goroutine tidak dijadwalkan berurutan, jadi potret lama bisa mendarat
// terakhir dan menetap — penerima tidak memeriksa nomor revisi) dan JUMLAH
// (impor besar melahirkan satu goroutine per baris, masing-masing menyusun
// muatan penuh).
//
// Slot berkapasitas satu dan penyiar membaca revisi TERKINI sendiri, jadi
// permintaan yang menumpuk dilebur jadi satu siaran berisi keadaan terbaru.
func (h *RealtimeHub) mulaiPenyiar() {
	h.sinyal = make(chan struct{}, 1)
	go func() {
		for range h.sinyal {
			h.broadcast(h.revision())
		}
	}()
}

// picu meminta siaran tanpa pernah memblokir pemanggilnya.
func (h *RealtimeHub) picu() {
	if h.sinyal == nil {
		return
	}
	select {
	case h.sinyal <- struct{}{}:
	default:
	}
}

// NewRealtimeHubData membuat hub yang menyiarkan ISI, bukan sekadar nomor
// revisi. Layar yang berlangganan hub ini memakai muatannya apa adanya — tidak
// ada permintaan HTTP susulan.
func NewRealtimeHubData(sumber string, muatan func() any) *RealtimeHub {
	h := &RealtimeHub{conns: map[*websocket.Conn]bool{}, muatan: muatan, sumber: sumber}
	h.mulaiPenyiar()
	return h
}

// Ikut menambahkan `data` sebagai pengikut hub ini: satu bump menyiarkan ke
// hub ini DAN ke setiap pengikut. Boleh dipanggil berkali-kali.
func (h *RealtimeHub) Ikut(data *RealtimeHub) { h.pengikut = append(h.pengikut, data) }

func (h *RealtimeHub) revision() int64 { return atomic.LoadInt64(&h.rev) }

// pesan menyusun badan siaran. Dipanggil SEKALI per siaran dan sengaja di luar
// kunci hub: muatan() membaca ulang daftar pekerjaan dan peringatan, dan
// menahan kunci selama itu memblokir siaran lain tanpa alasan.
func (h *RealtimeHub) pesan(rev int64) any {
	if h.muatan == nil {
		return map[string]int64{"rev": rev}
	}
	return map[string]any{"rev": rev, "divisi": h.sumber, "data": h.muatan()}
}

func (h *RealtimeHub) bump() {
	rev := atomic.AddInt64(&h.rev, 1)

	// KEDUANYA lewat penyiar, tidak ada yang disiarkan di jalur permintaan.
	// Menyiarkan langsung di sini berarti permintaan yang menulis menunggu
	// seluruh penulisan jaringan selesai — termasuk klien berjaringan buruk,
	// sampai tenggat 5 detik — dan untuk hub data, juga menunggu muatannya
	// disusun dari dua sumber.
	h.picu()
	for _, p := range h.pengikut {
		atomic.StoreInt64(&p.rev, rev)
		p.picu()
	}
}

// Bump pushes a new revision to every connected dashboard. Exported so
// background jobs (e.g. content-plan auto-sync) can trigger a live refresh
// without going through an HTTP request.
func (h *RealtimeHub) Bump() { h.bump() }

func (h *RealtimeHub) broadcast(rev int64) {
	// Daftar koneksi disalin di bawah kunci, lalu kuncinya DILEPAS sebelum satu
	// byte pun ditulis ke jaringan.
	h.mu.Lock()
	if len(h.conns) == 0 {
		h.mu.Unlock()
		return
	}
	daftar := make([]*websocket.Conn, 0, len(h.conns))
	for c := range h.conns {
		daftar = append(daftar, c)
	}
	h.mu.Unlock()

	// Muatan disusun SEBELUM kunci tulis diambil: pesan() memanggil muatan(),
	// yang menghitung ulang seluruh ringkasan divisi. Menahan kunci selama
	// perhitungan itu membuat sinkronisasi awal koneksi BARU ikut menunggu —
	// sendTo memakai kunci yang sama, padahal ia tidak butuh apa pun dari
	// siaran yang sedang berjalan.
	msg := h.pesan(rev)

	h.tulis.Lock()
	defer h.tulis.Unlock()
	var mati []*websocket.Conn
	for _, c := range daftar {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := c.WriteJSON(msg); err != nil {
			mati = append(mati, c)
		}
	}
	if len(mati) == 0 {
		return
	}
	h.mu.Lock()
	for _, c := range mati {
		delete(h.conns, c)
		_ = c.Close()
	}
	h.mu.Unlock()
}

// JedaWaktuBerlalu — lihat MulaiSiaranBerkala.
const JedaWaktuBerlalu = 60 * time.Second

// MulaiSiaranBerkala menyiarkan muatan tiap `jeda`, SENGAJA tanpa menunggu
// revisi berubah.
//
// Yang ditangkapnya adalah angka yang bergerak semata karena waktu lewat:
// konten yang menua melewati SLA, pekerjaan yang lewat tenggat. Tidak ada yang
// menulis apa pun saat itu terjadi, jadi revisi tidak naik dan tidak ada push —
// layar akan membeku di angka kemarin sampai kebetulan ada orang menyimpan
// sesuatu. Dulu lubang ini ditambal frontend dengan menarik ulang berkala;
// menyiarkannya dari sini membuat angka itu ikut datang lewat stream.
//
// Sekaligus jadi denyut nadi di lapisan aplikasi: ping/pong WebSocket membuktikan
// SOKETNYA hidup, siaran ini membuktikan DATANYA mengalir.
//
// Gratis saat sepi: broadcast() melewatkan penyusunan muatan kalau tidak ada
// yang menonton.
func (h *RealtimeHub) MulaiSiaranBerkala(jeda time.Duration) {
	go func() {
		t := time.NewTicker(jeda)
		defer t.Stop()
		for range t.C {
			h.picu() // lewat penyiar tunggal, bukan broadcast langsung
		}
	}()
}

func (h *RealtimeHub) add(c *websocket.Conn) {
	h.mu.Lock()
	h.conns[c] = true
	h.mu.Unlock()
}

func (h *RealtimeHub) remove(c *websocket.Conn) {
	h.mu.Lock()
	delete(h.conns, c)
	h.mu.Unlock()
	_ = c.Close()
}

func (h *RealtimeHub) sendTo(c *websocket.Conn, rev int64) {
	msg := h.pesan(rev) // di luar kunci, alasan sama seperti di broadcast

	// tulis, BUKAN mu: koneksi ini sudah terdaftar, jadi siaran yang sedang
	// berjalan bisa menulisi conn yang sama. Dua penulis pada satu conn dilarang
	// gorilla, dan akibatnya bingkai rusak.
	h.tulis.Lock()
	defer h.tulis.Unlock()
	_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_ = c.WriteJSON(msg)
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true }, // same-trust dev/LAN setup
}

// wsUpgraderData menyalakan permessage-deflate untuk soket data: muatan JSON
// dengan nama field berulang sangat mampat. Peramban menegosiasikannya sendiri;
// klien yang tidak mendukung tetap dilayani, hanya tanpa kompresi.
var wsUpgraderData = websocket.Upgrader{
	CheckOrigin:       func(*http.Request) bool { return true },
	EnableCompression: true,
}

// ServeWS upgrades the request to a WebSocket. Browsers cannot send the
// Authorization header on a WS handshake, so the bearer token is passed as a
// query parameter and validated with the token manager.
func (h *RealtimeHub) ServeWS(tm *middleware.TokenManager, sso *authmw.Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		tok := c.Query("token")
		if _, err := tm.Parse(tok); err != nil {
			if _, _, ok := middleware.SSOIdentity(sso, tok); !ok {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				return
			}
		}
		h.layani(&wsUpgrader, c)
	}
}

// ServeWSData adalah SOKET DATA: daftar pekerjaan + peringatan dini ikut di tiap
// pesan, sehingga layar memakainya langsung tanpa menarik ulang lewat HTTP.
//
// Izinnya sama ketatnya dengan ServeWS di atas — Marketing memang sudah memakai
// SSOIdentity (bukan varian Any), jadi di sini tidak ada pelonggaran yang perlu
// ditutup. Rutenya tetap dipisah supaya pemisahan "pemicu vs data" seragam di
// semua divisi, dan supaya melonggarkan /api/ws kelak tidak diam-diam ikut
// membuka datanya.
func (h *RealtimeHub) ServeWSData(tm *middleware.TokenManager, sso *authmw.Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		tok := c.Query("token")
		if _, err := tm.Parse(tok); err != nil {
			if _, _, ok := middleware.SSOIdentity(sso, tok); !ok {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				return
			}
		}
		h.layani(&wsUpgraderData, c)
	}
}

// layani mendaftarkan satu koneksi, mengirim sinkronisasi awal, lalu menyalakan
// read loop + ping loop. Dipakai kedua rute: yang membedakan mereka hanya
// pemeriksaan izin di atas dan upgrader yang dipilih.
func (h *RealtimeHub) layani(up *websocket.Upgrader, c *gin.Context) {
	conn, err := up.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	if up.EnableCompression {
		conn.EnableWriteCompression(true)
	}
	h.add(conn)
	// Sinkronisasi saat connect. Untuk soket data ini BUKAN formalitas: inilah
	// muatan pertama yang mengisi layar, sehingga halaman tidak perlu sekali pun
	// memanggil endpoint-endpoint sumbernya.
	h.sendTo(conn, h.revision())

	conn.SetReadLimit(512)
	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})
	go h.readLoop(conn)
	go h.pingLoop(conn)
}

// readLoop drains inbound frames (there are none of interest) and, crucially,
// lets the read deadline fire when a pong is missed — reaping dead connections.
func (h *RealtimeHub) readLoop(conn *websocket.Conn) {
	defer h.remove(conn)
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// pingLoop sends periodic pings so half-open connections are detected. WriteControl
// is safe to call concurrently with the WriteJSON broadcasts.
func (h *RealtimeHub) pingLoop(conn *websocket.Conn) {
	t := time.NewTicker(wsPingPeriod)
	defer t.Stop()
	for range t.C {
		if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(wsWriteWait)); err != nil {
			h.remove(conn)
			return
		}
	}
}

// BumpMiddleware bumps the revision after every successful mutating request.
func (h *RealtimeHub) BumpMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead &&
			c.Writer.Status() >= 200 && c.Writer.Status() < 300 {
			h.bump()
		}
	}
}
