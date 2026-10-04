package kafka

import (
	"bufio"
	"errors"
	"net"
	"net/http"
)

// statusRecorder menangkap kode status respons untuk EmitOnWrite, dan
// meneruskan Hijack/Flush supaya WebSocket dan streaming tetap hidup di balik
// pembungkus ini (pola yang sama dipakai statusRecorder di tiap service).
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying ResponseWriter does not support hijacking")
	}
	return hj.Hijack()
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// EmitOnWrite memancarkan gp.<source>.data "data.changed" setelah TIAP request
// tulis (non-GET/HEAD/OPTIONS) yang sukses (2xx) — satu titik yang mencakup
// semua tulisan service tanpa menyentuh handler satu per satu. Producer nil =
// pass-through murni (backbone mati).
//
// Key = "METHOD /path": resource sama → partisi sama → urutan per resource
// terjaga. Payload hanya method + path — isi data tetap di belakang endpoint
// ber-autentikasi; backbone TIDAK membocorkan data divisi mana pun.
//
// Dipakai di router.go tiap service, di samping bumpOnWrite lokal:
//
//	return chain(mux, logger, kafka.EmitOnWrite(h.events), bumpOnWrite(h.hub), cors(allowOrigin))
func EmitOnWrite(p *Producer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p == nil || isReadMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			if rec.status >= 200 && rec.status < 300 {
				p.EmitChanged(r.Method+" "+r.URL.Path, map[string]string{
					"method": r.Method,
					"path":   r.URL.Path,
				})
			}
		})
	}
}