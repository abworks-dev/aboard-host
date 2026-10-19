// Command check_unpatched_guard is a synthetic negative control. Run with the
// original fixed module pair, deliberately WITHOUT the compatibility modfile.
// It never starts a Host and must reject before creating a database.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	allowed := filepath.Join(cwd, ".data", "logs", "001-validation") + string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(os.TempDir())+string(os.PathSeparator), allowed) {
		panic("negative control requires the project-local synthetic temporary root")
	}
	root, err := os.MkdirTemp("", "unpatched-guard-")
	if err != nil {
		panic(err)
	}
	path := filepath.Join(root, "must-not-exist.sqlite")
	h, openErr := sqlite.Open(context.Background(), path, sqlite.CreateNew)
	if h != nil {
		_ = h.Close()
		panic("unsafe build obtained a SQLite handle")
	}
	if !errors.Is(openErr, sqlite.ErrProfile) {
		panic(fmt.Sprintf("missing fail-closed profile error: %v", openErr))
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		panic(fmt.Sprintf("unsafe build created a DB artifact: %v", err))
	}
	fmt.Printf("PASS: original unpatched pair rejected before DB creation: %v\n", openErr)
}
