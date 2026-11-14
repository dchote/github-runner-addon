package runtime

import (
	"os"
	"testing"
	"time"
)

func TestParseImageRefreshDays(t *testing.T) {
	if got := ParseImageRefreshDays(""); got != 7*24*time.Hour {
		t.Fatalf("empty: %s", got)
	}
	if got := ParseImageRefreshDays("14"); got != 14*24*time.Hour {
		t.Fatalf("14: %s", got)
	}
	if got := ParseImageRefreshDays("0"); got != 0 {
		t.Fatalf("off: %s", got)
	}
	if got := ParseImageRefreshDays("nope"); got != 7*24*time.Hour {
		t.Fatalf("invalid: %s", got)
	}
	if got := ParseImageRefreshDays("9999"); got != 365*24*time.Hour {
		t.Fatalf("cap: %s", got)
	}
}

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
