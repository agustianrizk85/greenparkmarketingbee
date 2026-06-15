package database

import (
	"fmt"

	"marketingflow/internal/config"
	"marketingflow/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
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
	); err != nil {
		return nil, err
	}
	return db, nil
}
