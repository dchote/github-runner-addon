package docker

import (
	"testing"
)

func TestStripBOM(t *testing.T) {
	raw := []byte{0xEF, 0xBB, 0xBF, '{', '}'}
	got := stripBOM(raw)
	if string(got) != "{}" {
		t.Fatalf("%q", got)
	}
	if string(stripBOM([]byte(`{"a":1}`))) != `{"a":1}` {
		t.Fatal("no-bom unchanged")
	}
}
