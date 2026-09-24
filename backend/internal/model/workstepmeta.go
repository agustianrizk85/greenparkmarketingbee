package model

// WorkStepMeta adalah SATU isian bebas milik sebuah langkah alur: tautan brief,
// tautan footage iCloud, tanggal shooting, platform top-up, tautan Meta Ads, dan
// seterusnya. Kuncinya bukan karangan bebas — tiap langkah mendaftarkan kunci
// yang boleh dipakainya di service/catalog.go (MetadataKeys).
//
// Sampai sebelum ini semuanya tinggal di SATU kolom `work_steps.metadata`
// bertipe jsonb. Isinya memang kantong kunci-nilai, jadi kolom itu selalu jadi
// tempat penampungan yang tidak bisa ditanyai: mencari "langkah mana saja yang
// tautan Meta Ads-nya masih kosong" berarti membongkar JSON tiap baris, dan
// tidak ada satu pun index yang bisa menolong.
//
// Sekarang tiap isian jadi BARIS. Pertanyaan seperti di atas jadi satu WHERE.
//
// Nilainya disimpan sebagai teks, dan itu cocok dengan kenyataannya: seluruh
// kunci yang terdaftar berisi tautan, tanggal, atau nama platform, dan layar
// membacanya lewat String() tanpa kecuali.
type WorkStepMeta struct {
	// Dua kolom ini bersama jadi kunci utama: satu langkah tidak mungkin punya
	// dua nilai untuk kunci yang sama, dan menegakkannya di database berarti
	// tidak perlu mempercayai setiap jalur tulis untuk mengingatnya.
	WorkStepID uint   `gorm:"primaryKey" json:"work_step_id"`
	Kunci      string `gorm:"primaryKey;size:64" json:"kunci"`
	Nilai      string `gorm:"type:text" json:"nilai"`
}

func (WorkStepMeta) TableName() string { return "work_step_meta" }
