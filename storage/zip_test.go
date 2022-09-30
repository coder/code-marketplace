package storage

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

var files = []struct {
	Name, Body string
}{
	{"alpha.txt", "Alpha content."},
	{"beta.txt", "Beta content."},
	{"charlie.txt", "Charlie content."},
}

func createZip() ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	zw := zip.NewWriter(buf)
	for _, file := range files {
		fw, err := zw.Create(file.Name)
		if err != nil {
			return nil, err
		}
		if _, err := fw.Write([]byte(file.Body)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func TestGetZipFileReader(t *testing.T) {
	t.Parallel()

	buffer, err := createZip()
	require.NoError(t, err)

	for _, file := range files {
		reader, err := GetZipFileReader(buffer, file.Name)
		require.NoError(t, err)

		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.Equal(t, file.Body, string(content))
	}

	_, err = GetZipFileReader(buffer, "delta.txt")
	require.Error(t, err)
}

type nopCloser struct {
	io.Writer
}

func (nopCloser) Close() error { return nil }

func TestExtract(t *testing.T) {
	t.Parallel()

	buffer, err := createZip()
	require.NoError(t, err)

	t.Run("Error", func(t *testing.T) {
		err := ExtractZip(buffer, func(name string) (io.WriteCloser, error) {
			return nil, errors.New("error")
		})
		require.Error(t, err)
	})

	t.Run("OK", func(t *testing.T) {
		called := []string{}
		err := ExtractZip(buffer, func(name string) (io.WriteCloser, error) {
			called = append(called, name)
			return nopCloser{io.Discard}, nil
		})
		require.NoError(t, err)
		require.Equal(t, []string{"alpha.txt", "beta.txt", "charlie.txt"}, called)
	})
}
