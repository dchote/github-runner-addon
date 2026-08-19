package runtime

import (
	"os"
	"testing"
)

func TestAddrListenOverride(t *testing.T) {
	c := Config{HTTPPort: "8099", ListenAddr: "0.0.0.0:8099"}
	if got := c.Addr(); got != "0.0.0.0:8099" {
		t.Fatalf("got %q", got)
	}
}

func TestAddrSupervisorBindsAll(t *testing.T) {
	t.Setenv("SUPERVISOR_TOKEN", "x")
	c := Config{HTTPPort: "8099"}
	if got := c.Addr(); got != ":8099" {
		t.Fatalf("got %q", got)
	}
}

func TestAddrDefaultWithoutContainerHints(t *testing.T) {
	t.Setenv("SUPERVISOR_TOKEN", "")
	c := Config{HTTPPort: "8099"}
	got := c.Addr()
	if _, err := os.Stat("/.dockerenv"); err == nil {
		if got != ":8099" {
			t.Fatalf("in docker: %q", got)
		}
		return
	}
	if got != "127.0.0.1:8099" {
		t.Fatalf("host bind: %q", got)
	}
}
