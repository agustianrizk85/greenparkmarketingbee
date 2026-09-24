package database

import (
	"encoding/json"
	"fmt"
	"log"

	"marketingflow/internal/config"
	"marketingflow/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// Connect opens the database connection (PostgreSQL or SQLite) and runs
// auto-migration. The driver is selected via DB_DRIVER so the same codebase
// runs against production Postgres or a zero-setup local SQLite file.
func Connect(cfg *config.Config) (*gorm.DB, error) {
	logLevel := logger.Warn
	if cfg.AppEnv == "development" {
		logLevel = logger.Info
	}
	gormCfg := &gorm.Config{Logger: logger.Default.LogMode(logLevel)}

	var (
		db  *gorm.DB
		err error
	)
	switch cfg.DBDriver {
	case "sqlite":
		db, err = gorm.Open(sqlite.Open(cfg.DBPath), gormCfg)
	case "postgres":
		db, err = gorm.Open(postgres.Open(cfg.DSN()), gormCfg)
	default:
		return nil, fmt.Errorf("unsupported DB_DRIVER %q (use postgres or sqlite)", cfg.DBDriver)
	}
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.WorkItem{},
		&model.WorkStep{},
		&model.Document{},
		&model.MetaAppConfig{},
		&model.MetaConnection{},
		// Tautan proyek konten -> proyek Perencanaan (sambungan lintas divisi).
		&model.ProjectLink{},
		// Percakapan antar tim pada satu langkah alur konten.
		&model.StepComment{},
		// Perintah yang lahir dari War Room, beserta cara memverifikasinya.
		&model.WarroomKeputusan{},
		// Isian bebas per langkah (tautan brief, footage, Meta Ads, tanggal…).
		// Dulu satu kolom jsonb `work_steps.metadata`; lihat WorkStepMeta.
		&model.WorkStepMeta{},
	); err != nil {
		return nil, err
	}

	// Perbaikan sekali-jalan untuk data lama: sebelum SourceKey jadi nullable,
	// item manual disimpan dengan kunci "" dan hanya SATU yang bisa masuk.
	// Tanpa baris ini, basis data lama tetap menolak item manual berikutnya
	// walau kodenya sudah benar. Aman diulang: setelah kosong, tidak ada lagi
	// baris yang cocok.
	if err := db.Model(&model.WorkItem{}).
		Where("source_key = ?", "").
		Update("source_key", nil).Error; err != nil {
		return nil, err
	}

	if err := pindahkanMetadata(db); err != nil {
		return nil, err
	}
	return db, nil
}

// pindahkanMetadata memindahkan isi kolom lama `work_steps.metadata` (jsonb)
// ke tabel work_step_meta, satu baris per kunci.
//
// Dijalankan saat start, bukan lewat perintah terpisah, karena repo ini tidak
// punya deploy.sh: kalau perpindahannya menuntut langkah manual, ia akan
// terlewat, dan yang terlihat adalah seluruh isian langkah mendadak kosong.
//
// Aman diulang, dan itu ditegakkan dua lapis: ia berhenti kalau tabel barunya
// sudah berisi, dan penulisannya sendiri melewati baris yang kuncinya sudah ada.
//
// Kolom lamanya TIDAK dihapus. Selama ia masih di sana, mundur cukup dengan
// mengembalikan kodenya — tidak ada data yang perlu dipulihkan dari cadangan.
func pindahkanMetadata(db *gorm.DB) error {
	// Kolom lama sudah tidak dikenal model, jadi keberadaannya harus ditanyakan
	// ke database. Di pemasangan baru ia memang tidak pernah ada.
	if !db.Migrator().HasColumn(&model.WorkStep{}, "metadata") {
		return nil
	}
	var sudah int64
	if err := db.Model(&model.WorkStepMeta{}).Count(&sudah).Error; err != nil {
		return err
	}
	if sudah > 0 {
		return nil
	}

	var rows []struct {
		ID       uint
		Metadata []byte
	}
	if err := db.Table("work_steps").
		Select("id, metadata").
		Where("metadata IS NOT NULL").
		Find(&rows).Error; err != nil {
		return err
	}

	keluar := []model.WorkStepMeta{}
	for _, r := range rows {
		if len(r.Metadata) == 0 {
			continue
		}
		var isi map[string]any
		if err := json.Unmarshal(r.Metadata, &isi); err != nil {
			// Satu baris rusak tidak boleh menggagalkan start: yang lain masih
			// bisa dipindahkan, dan barisnya tetap utuh di kolom lama untuk
			// diperiksa orang.
			log.Printf("marketingflow: metadata langkah %d tidak terbaca, dilewati: %v", r.ID, err)
			continue
		}
		for k, v := range isi {
			if v == nil {
				continue
			}
			s, ok := v.(string)
			if !ok {
				s = fmt.Sprint(v)
			}
			keluar = append(keluar, model.WorkStepMeta{WorkStepID: r.ID, Kunci: k, Nilai: s})
		}
	}
	if len(keluar) == 0 {
		return nil
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&keluar).Error; err != nil {
		return err
	}
	log.Printf("marketingflow: %d isian metadata dipindah dari kolom jsonb ke work_step_meta", len(keluar))
	return nil
}
