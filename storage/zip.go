package storage

import (
	"archive/zip"
	"bytes"
	"io"

	"golang.org/x/xerrors"
)

// WalkZip applies a function over every file in the zip. If the function
// returns true a reader for that file will be immediately returned. If it
// returns an error the error will immediately be returned. Otherwise `nil` will
// be returned once the archive's end is reached.
func WalkZip(rawZip []byte, fn func(*zip.File) (bool, error)) (io.ReadCloser, error) {
	b := bytes.NewReader(rawZip)
	zr, err := zip.NewReader(b, b.Size())
	if err != nil {
		return nil, err
	}
	for _, zf := range zr.File {
		stop, err := fn(zf)
		if err != nil {
			return nil, err
		}
		if stop {
			zfr, err := zf.Open()
			if err != nil {
				return nil, err
			}
			return zfr, nil
		}
	}
	return nil, nil
}

// GetZipFileReader returns a reader for a single file in a zip.
func GetZipFileReader(rawZip []byte, filename string) (io.ReadCloser, error) {
	reader, err := WalkZip(rawZip, func(f *zip.File) (stop bool, err error) {
		return f.Name == filename, nil
	})
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return nil, xerrors.Errorf("%s not found", filename)
	}
	return reader, nil
}

// ExtractZip applies a function with a reader for every file in the zip.  If
// the function returns an error the walk is aborted.
func ExtractZip(rawZip []byte, fn func(name string, reader io.Reader) error) error {
	_, err := WalkZip(rawZip, func(zf *zip.File) (stop bool, err error) {
		if !zf.FileInfo().IsDir() {
			zr, err := zf.Open()
			if err != nil {
				return false, err
			}
			defer zr.Close()
			return false, fn(zf.Name, zr)
		}
		return false, nil
	})

	return err
}
