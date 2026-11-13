package state

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/mythologyli/zju-connect/internal/identity"
	"github.com/mythologyli/zju-connect/internal/securefile"
)

func TestExclusiveLockAndSessionBinding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "customer")
	d, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if second, err := Acquire(path); !errors.Is(err, ErrLocked) {
		if second != nil {
			second.Close()
		}
		t.Fatalf("duplicate lock: %v", err)
	}
	binding := identity.Binding{Namespace: "tinyrill/v1", Server: "a.example:443", Customer: "a"}
	want := []byte(`{"cookie":"private-test-value"}`)
	if err := d.SaveSession(binding, want); err != nil {
		t.Fatal(err)
	}
	if err := securefile.Check(filepath.Join(path, "session.json")); err != nil {
		t.Fatal(err)
	}
	got, err := d.LoadSession(binding)
	if err != nil || string(got) != string(want) {
		t.Fatalf("load error: %v", err)
	}
	other := binding
	other.Server = "b.example:443"
	if _, err := d.LoadSession(other); !errors.Is(err, ErrBinding) {
		t.Fatalf("changed identity: %v", err)
	}
	if err := d.ClearSession(); err != nil {
		t.Fatal(err)
	}
	if got, err := d.LoadSession(binding); err != nil || got != nil {
		t.Fatal("clear failed")
	}
	d.Close()
	second, err := Acquire(path)
	if err != nil {
		t.Fatalf("released lock: %v", err)
	}
	second.Close()
}
