package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration loaded from the environment.
type Config struct {
	AppPort string
	AppEnv  string

	DBDriver   string // "postgres" or "sqlite"
	DBPath     string // sqlite file path
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	JWTSecret      string
	JWTExpiryHours int

	UploadDir string

	SeedKadepPassword  string
	SeedStaffPassword  string
	SeedViewerPassword string
}

// Load reads configuration from a .env file (if present) and the environment.
func Load() *Config {
	// .env is optional; ignore the error when it is absent.
	_ = godotenv.Load()

	return &Config{
		AppPort: getEnv("APP_PORT", "8086"),
		AppEnv:  getEnv("APP_ENV", "development"),

		DBDriver:   getEnv("DB_DRIVER", "sqlite"),
		DBPath:     getEnv("DB_PATH", "./marketingflow.db"),
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "postgres"),
		DBPassword: getEnv("DB_PASSWORD", "postgres"),
		DBName:     getEnv("DB_NAME", "marketingflow"),
		DBSSLMode:  getEnv("DB_SSLMODE", "disable"),

		JWTSecret:      getEnv("JWT_SECRET", "dev-secret"),
		JWTExpiryHours: getEnvInt("JWT_EXPIRY_HOURS", 12),

		UploadDir: getEnv("UPLOAD_DIR", "./uploads"),

		SeedKadepPassword:  getEnv("SEED_KADEP_PASSWORD", "kadep123"),
		SeedStaffPassword:  getEnv("SEED_STAFF_PASSWORD", "staff123"),
		SeedViewerPassword: getEnv("SEED_VIEWER_PASSWORD", "viewer123"),
	}
}

// DSN builds the PostgreSQL connection string.
func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=Asia/Jakarta",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode,
	)
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
