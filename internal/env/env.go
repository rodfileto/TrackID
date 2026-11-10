// Package env loads .env files for the backend's command-line tools, and reads secrets from files.
package env

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Load reads .env from the current directory or, failing that, the parent directory — so it
// works whether a command is run from the repo root (where go.mod lives) or from a subdirectory.
// Both misses are silently ignored: .env is a local dev convenience, not a requirement (env vars
// set directly in the shell or process environment still work). Then it reads the secrets given
// as files (ResolveFiles), and stops the process if one can't be read.
func Load() {
	if err := godotenv.Load(".env"); err != nil {
		_ = godotenv.Load("../.env")
	}
	if err := ResolveFiles(); err != nil {
		log.Fatal(err)
	}
}

// Secrets are the variables that may be given as a file instead, as <NAME>_FILE: the path of a
// file holding the value, the convention of Docker and Kubernetes secrets. The value then never
// sits in .env or in the process's environment as the container was started. The list is explicit
// because other variables already end in _FILE and name ordinary files.
var Secrets = []string{
	"DATABASE_URL",
	"REDIS_URL",
	"JWT_SECRET",
	"S3_ACCESS_KEY",
	"S3_SECRET_KEY",
	"SECRETS_KEY",
	"ASSIST_API_KEY",
	"ASK_API_KEY",
	"TRACKID_SUPERUSER_PASSWORD",
}

// ResolveFiles sets each of Secrets whose <NAME>_FILE is set from that file, without the trailing
// newline an editor or `echo` leaves. Setting both NAME and NAME_FILE is an error, since it isn't
// clear which is meant; so is a file that can't be read or is empty.
func ResolveFiles() error {
	for _, name := range Secrets {
		path := os.Getenv(name + "_FILE")
		if path == "" {
			continue
		}
		if os.Getenv(name) != "" {
			return fmt.Errorf("both %s and %s_FILE are set: set one", name, name)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s_FILE: %w", name, err)
		}
		value := strings.TrimRight(string(content), "\r\n")
		if value == "" {
			return fmt.Errorf("%s_FILE: %s is empty", name, path)
		}
		if err := os.Setenv(name, value); err != nil {
			return err
		}
	}
	return nil
}
