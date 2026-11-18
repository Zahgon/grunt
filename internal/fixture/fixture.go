// Package fixture reads the gzip-compressed JSON files, recorded from Node, that the
// package tests replay.
package fixture

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
)

// ReadGzip returns the decompressed contents of the gzip file at path.
func ReadGzip(path string) ([]byte, error) {
	compressed, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(zr)
	if cerr := zr.Close(); err == nil {
		err = cerr
	}
	return data, err
}

// DecodeGzipJSON decodes the JSON document in the gzip file at path into v.
func DecodeGzipJSON(path string, v any) error {
	data, err := ReadGzip(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
