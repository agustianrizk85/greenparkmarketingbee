package config

import "testing"

func TestGetEnvList(t *testing.T) {
	fallback := []string{"a", "b"}

	// Unset → fallback.
	t.Setenv("CORS_TEST", "")
	if got := getEnvList("CORS_TEST", fallback); len(got) != 2 || got[0] != "a" {
		t.Errorf("empty env: got %v, want fallback %v", got, fallback)
	}

	// Set → split, trimmed, blanks dropped.
	t.Setenv("CORS_TEST", " https://a.com , , https://b.com ")
	got := getEnvList("CORS_TEST", fallback)
	want := []string{"https://a.com", "https://b.com"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSecurityWarnings(t *testing.T) {
	// Development env: warnings suppressed regardless of defaults.
	dev := &Config{AppEnv: "development", JWTSecret: "dev-secret", SeedKadepPassword: "kadep123"}
	if w := dev.SecurityWarnings(); len(w) != 0 {
		t.Errorf("development: expected no warnings, got %v", w)
	}

	// Production with insecure defaults: must warn about the JWT secret at least.
	prod := &Config{
		AppEnv:             "production",
		JWTSecret:          "dev-secret",
		SeedKadepPassword:  "kadep123",
		SeedStaffPassword:  "staff123",
		SeedViewerPassword: "viewer123",
	}
	if w := prod.SecurityWarnings(); len(w) == 0 {
		t.Error("production with defaults: expected warnings, got none")
	}

	// Production, properly configured: no warnings.
	secure := &Config{
		AppEnv:             "production",
		JWTSecret:          "a-strong-random-secret",
		SeedKadepPassword:  "x9$kad",
		SeedStaffPassword:  "x9$stf",
		SeedViewerPassword: "x9$vwr",
	}
	if w := secure.SecurityWarnings(); len(w) != 0 {
		t.Errorf("secure production: expected no warnings, got %v", w)
	}
}
