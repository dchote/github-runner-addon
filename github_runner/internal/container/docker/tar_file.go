package docker

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
)

// tarRegularFile builds a one-file ustar archive for CopyToContainer.
func tarRegularFile(name string, data []byte) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name: name,
		Mode: 0o600,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, err
	}
	if _, err := tw.Write(data); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return &buf, nil
}

// readTarRegularFile reads the first regular file entry from a Docker CopyFromContainer tar stream.
// notFound is returned (wrapped with label) when the archive is empty.
func readTarRegularFile(r io.Reader, notFound error, label string) ([]byte, error) {
	tr := tar.NewReader(r)
	hdr, err := tr.Next()
	if errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: %s", notFound, label)
	}
	if err != nil {
		return nil, err
	}
	if hdr.Typeflag != tar.TypeReg {
		return nil, fmt.Errorf("%s is not a regular file", label)
	}
	data, err := io.ReadAll(tr)
	if err != nil {
		return nil, err
	}
	return data, nil
}
