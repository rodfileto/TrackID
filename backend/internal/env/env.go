// Package env loads .env files for the backend's command-line tools.
package env

import "github.com/joho/godotenv"

// Load reads .env from the current directory or, failing that, the parent directory — so it
// works whether a command is run from the repo root or from backend/, where go.mod lives. Both
// misses are silently ignored: .env is a local dev convenience, not a requirement (env vars set
// directly in the shell or process environment still work).
func Load() {
	if err := godotenv.Load(".env"); err == nil {
		return
	}
	_ = godotenv.Load("../.env")
}
