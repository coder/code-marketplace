package storage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/code-marketplace/storage"
)

func localFactory(t *testing.T) testStorage {
	extdir := t.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	s, err := storage.NewLocalStorage(extdir, logger)
	require.NoError(t, err)
	return testStorage{
		storage: s,
		write: func(content []byte, elem ...string) {
			dest := filepath.Join(extdir, filepath.Join(elem...))
			err := os.MkdirAll(filepath.Dir(dest), 0o755)
			require.NoError(t, err)
			err = os.WriteFile(dest, content, 0o644)
			require.NoError(t, err)
		},
		exists: func(elem ...string) bool {
			_, err := os.Stat(filepath.Join(extdir, filepath.Join(elem...)))
			return err == nil
		},
	}
}
