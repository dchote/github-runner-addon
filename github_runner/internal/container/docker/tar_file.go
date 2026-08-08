package docker

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
)

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
