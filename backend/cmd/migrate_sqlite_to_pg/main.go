// Command migrate_sqlite_to_pg menyalin SELURUH data marketingflow dari file
// SQLite lama (DB_PATH, mis. ./marketingflow.db) ke database PostgreSQL tujuan.
// Pola sama dengan be/permit/cmd/migrate_sqlite_to_pg.
//
// Dijalankan SEKALI saat pindah dari SQLite ke Postgres. Aman diulang
// (idempoten: ON CONFLICT DO NOTHING). ID dipertahankan agar relasi antar-tabel
// (work item↔langkah↔dokumen↔komentar) tetap konsisten; FK dimatikan
// sementara (session_replication_role=replica, butuh superuser) dan sequence
// di-reset ke MAX(id) setelah salin agar insert berikutnya tak bentrok.
//
//	SQLite sumber : $DB_PATH            (default ./marketingflow.db)
//	Postgres tujuan: $PG_DSN            (default host=localhost port=5432 db=marketingflow)
//
// Contoh:
//	cd backend && go run ./cmd/migrate_sqlite_to_pg
package main

import (
	"fmt"
	"log"
	"os"
	"reflect"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	"marketingflow/internal/model"
)

// models: SAMA dengan AutoMigrate di internal/database/database.go
// (parent dulu). FK dimatikan saat salin, jadi urutan hanya jaga-jaga.
// Model baru di database.go WAJIB ditambahkan juga di sini — yang tak terdaftar
// tidak ikut tersalin, dan tidak ada peringatan apa pun.
func models() []any {
	return []any{
		&model.User{},
		&model.WorkItem{},
		&model.WorkStep{},
		&model.Document{},
		&model.MetaAppConfig{},
		&model.MetaConnection{},
		&model.ProjectLink{},
		&model.StepComment{},
		&model.WarroomKeputusan{},
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func must(err error, what string) {
	if err != nil {
		log.Fatalf("FATAL %s: %v", what, err)
	}
}

func main() {
	sqlitePath := env("DB_PATH", "./marketingflow.db")
	pgDSN := env("PG_DSN", "host=localhost user=postgres password=postgres dbname=marketingflow port=5432 sslmode=disable")

	if _, err := os.Stat(sqlitePath); err != nil {
		log.Fatalf("SQLite sumber tak ditemukan di %q: %v", sqlitePath, err)
	}

	gcfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	src, err := gorm.Open(sqlite.Open(sqlitePath), gcfg)
	must(err, "buka SQLite sumber")
	dst, err := gorm.Open(postgres.Open(pgDSN), gcfg)
	must(err, "buka Postgres tujuan")

	// Satu koneksi saja supaya `SET session_replication_role` berlaku untuk
	// SEMUA operasi salin (kalau pool, setting bisa kena koneksi lain).
	sqlDB, err := dst.DB()
	must(err, "ambil *sql.DB tujuan")
	sqlDB.SetMaxOpenConns(1)

	log.Printf("SQLite : %s", sqlitePath)
	log.Printf("Postgres: %s", pgDSN)

	must(dst.AutoMigrate(models()...), "AutoMigrate Postgres")

	// Matikan FK sementara (butuh superuser; user postgres = superuser).
	must(dst.Exec("SET session_replication_role = replica").Error, "matikan FK")

	total := 0
	for _, m := range models() {
		n, err := copyModel(src, dst, m)
		if err != nil {
			log.Printf("  WARN salin %-16s: %v", tableOf(dst, m), err)
			continue
		}
		total += n
		log.Printf("  ✓ %-16s %d baris", tableOf(dst, m), n)
	}

	must(dst.Exec("SET session_replication_role = DEFAULT").Error, "aktifkan FK")

	// Reset sequence tiap tabel ke MAX(id) supaya insert baru tak tabrakan.
	for _, m := range models() {
		resetSeq(dst, m)
	}

	log.Printf("SELESAI — total %d baris tersalin ke Postgres.", total)
}

// copyModel membaca semua baris satu model dari SQLite lalu menulisnya ke
// Postgres (batch, pertahankan PK, ON CONFLICT DO NOTHING → aman diulang).
func copyModel(src, dst *gorm.DB, m any) (int, error) {
	t := reflect.TypeOf(m).Elem()
	slicePtr := reflect.New(reflect.SliceOf(t)) // *[]T
	if err := src.Find(slicePtr.Interface()).Error; err != nil {
		return 0, err
	}
	n := slicePtr.Elem().Len()
	if n == 0 {
		return 0, nil
	}
	err := dst.Clauses(clause.OnConflict{DoNothing: true}).
		Session(&gorm.Session{SkipHooks: true, FullSaveAssociations: false}).
		CreateInBatches(slicePtr.Interface(), 200).Error
	return n, err
}

func tableOf(db *gorm.DB, m any) string {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(m); err != nil {
		return fmt.Sprintf("%T", m)
	}
	return stmt.Schema.Table
}

// resetSeq menyetel sequence PK tabel ke MAX(id) (hanya untuk PK serial/int).
func resetSeq(db *gorm.DB, m any) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(m); err != nil || stmt.Schema == nil {
		return
	}
	table := stmt.Schema.Table
	if len(stmt.Schema.PrimaryFields) != 1 {
		return
	}
	pk := stmt.Schema.PrimaryFields[0].DBName
	var seq *string
	if err := db.Raw("SELECT pg_get_serial_sequence(?, ?)", table, pk).Scan(&seq).Error; err != nil || seq == nil || *seq == "" {
		return
	}
	db.Exec(fmt.Sprintf("SELECT setval('%s', COALESCE((SELECT MAX(%s) FROM %q), 1), true)", *seq, pk, table))
}
