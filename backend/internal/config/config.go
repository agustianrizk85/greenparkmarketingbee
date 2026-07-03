package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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

	// CORSOrigins lists the browser origins allowed to call the API. Overridable
	// via CORS_ORIGINS (comma-separated) so production origins aren't hardcoded.
	CORSOrigins []string

	UploadDir string

	SeedKadepPassword  string
	SeedStaffPassword  string
	SeedViewerPassword string

	// Meta (Facebook) Graph API — powers the Ads / WhatsApp / Instagram tabs.
	// MetaToken is the legacy single-token fallback used only when no OAuth
	// account is connected; the connected accounts (DB) take precedence.
	MetaToken      string
	MetaBusinessID string
	MetaAPIVersion string
	MetaAdAccount  string // pin a specific ad account id (without act_); empty = auto-pick by spend

	// Meta OAuth (Facebook Login) — credentials are normally entered from the UI
	// and stored in the DB; these env values only seed the DB on first run.
	MetaAppID       string
	MetaAppSecret   string
	MetaRedirectURI string // OAuth callback; must match the Meta App registration
	MetaScopes      string // comma-separated default scopes requested at login

	// Content Plan sync (Google Sheets → work items). Credentials empty => the
	// sheet is read via its public XLSX export (must be link-viewable).
	ContentSheetID    string // source spreadsheet id ("Content Plan GP 2026")
	GoogleCredentials []byte // optional service-account JSON for private sheets
}

// defaultContentSheetID is the "Content Plan GP 2026" spreadsheet.
const defaultContentSheetID = "1BKEY3j2BPIk6DOrfhvMbYj4CoABCLAQcZd6upMXbLOw"

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

		CORSOrigins: getEnvList("CORS_ORIGINS", []string{
			"http://localhost:5173", "http://localhost:5174",
			"http://localhost:5177", "http://localhost:3000",
		}),

		UploadDir: getEnv("UPLOAD_DIR", "./uploads"),

		SeedKadepPassword:  getEnv("SEED_KADEP_PASSWORD", "kadep123"),
		SeedStaffPassword:  getEnv("SEED_STAFF_PASSWORD", "staff123"),
		SeedViewerPassword: getEnv("SEED_VIEWER_PASSWORD", "viewer123"),

		MetaToken:      getEnv("META_ACCESS_TOKEN", ""),
		MetaBusinessID: getEnv("META_BUSINESS_ID", "146016010006333"),
		MetaAPIVersion: getEnv("META_API_VERSION", "v21.0"),
		MetaAdAccount:  getEnv("META_AD_ACCOUNT_ID", ""),

		MetaAppID:       getEnv("META_APP_ID", ""),
		MetaAppSecret:   getEnv("META_APP_SECRET", ""),
		MetaRedirectURI: getEnv("META_REDIRECT_URI", "http://localhost:8086/api/meta/oauth/callback"),
		MetaScopes:      getEnv("META_SCOPES", "ads_read,pages_show_list,instagram_basic,instagram_manage_messages,whatsapp_business_management,business_management"),

		ContentSheetID:    getEnv("CONTENT_SHEET_ID", defaultContentSheetID),
		GoogleCredentials: loadGoogleCredentials(),
	}
}

// loadGoogleCredentials reads the service-account JSON from MARKETING_GOOGLE_CREDENTIALS
// (inline JSON or a path), else a "google-credentials.json" file next to the
// executable or in the working directory. Empty result => public-export sync.
func loadGoogleCredentials() []byte {
	if v := strings.TrimSpace(os.Getenv("MARKETING_GOOGLE_CREDENTIALS")); v != "" {
		if strings.HasPrefix(v, "{") {
			return []byte(v) // inline JSON
		}
		if b, err := os.ReadFile(v); err == nil { // path
			return b
		}
	}
	candidates := []string{"google-credentials.json"}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "google-credentials.json"))
	}
	for _, p := range candidates {
		if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
			return b
		}
	}
	return nil
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

// getEnvList reads a comma-separated env var into a trimmed, non-empty slice,
// falling back to the given default when unset/empty.
func getEnvList(key string, fallback []string) []string {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

// SecurityWarnings returns human-readable warnings about insecure defaults that
// are acceptable in development but dangerous in production. Empty in a properly
// configured deployment. main logs these at startup so misconfig is visible.
func (c *Config) SecurityWarnings() []string {
	if strings.EqualFold(c.AppEnv, "development") {
		return nil
	}
	var w []string
	if c.JWTSecret == "" || c.JWTSecret == "dev-secret" {
		w = append(w, "JWT_SECRET is the insecure default — set a strong secret (tokens are forgeable otherwise).")
	}
	for k, v := range map[string]string{
		"SEED_KADEP_PASSWORD":  c.SeedKadepPassword,
		"SEED_STAFF_PASSWORD":  c.SeedStaffPassword,
		"SEED_VIEWER_PASSWORD": c.SeedViewerPassword,
	} {
		if v == "kadep123" || v == "staff123" || v == "viewer123" {
			w = append(w, k+" is a default demo password — override it before first run.")
		}
	}
	return w
}

func getEnvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
