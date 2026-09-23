package config

import (
	"os"
	"path/filepath"
)

// GUDANG BERKAS BERSAMA — semua divisi Greenpark menaruh file unggahan di SATU
// pohon folder, bukan tersebar di folder masing-masing backend. Dulu folder ini
// relatif terhadap working directory (`./uploads`), jadi menjalankan binary dari
// folder lain membuat dokumen yang sudah diunggah seolah hilang — dan
// mencadangkan berkas perusahaan berarti berburu ke 7 tempat berbeda.
//
// Akarnya diatur env `GP_FILE_DIR`. Kalau kosong dipakai path laptop dev di
// bawah — di server produksi (Linux) env ini WAJIB di-set, karena path Windows
// ini tidak akan pernah ada di sana.
//
// Salinan fungsi yang sama ada di tiap modul Go (auth, permit, perencanaan,
// teknik, metaapi) — mereka modul terpisah tanpa paket bersama, jadi tidak bisa
// mengimpor yang ini.
const defaultFileRoot = `C:\Users\PC\greenpark\be\auth\data\file`

// GPFileDir mengembalikan (dan menyiapkan) sub-folder `sub` di gudang bersama.
func GPFileDir(sub string) string {
	root := os.Getenv("GP_FILE_DIR")
	if root == "" {
		root = defaultFileRoot
	}
	dir := filepath.Join(root, sub)
	_ = os.MkdirAll(dir, 0o755)
	return dir
}
