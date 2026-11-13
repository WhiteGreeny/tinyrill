package credentials

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mythologyli/zju-connect/internal/identity"
	"github.com/mythologyli/zju-connect/internal/securefile"
)

type memoryBackend struct {
	data    map[string][]byte
	failure error
}

func (b *memoryBackend) Get(_ context.Context, k string) ([]byte, error) {
	if b.failure != nil {
		return nil, b.failure
	}
	v, ok := b.data[k]
	if !ok {
		return nil, ErrNotFound
	}
	return bytes.Clone(v), nil
}
func (b *memoryBackend) Set(_ context.Context, k string, v []byte) error {
	if b.failure != nil {
		return b.failure
	}
	b.data[k] = bytes.Clone(v)
	return nil
}
func (b *memoryBackend) Delete(_ context.Context, k string) error {
	if b.failure != nil {
		return b.failure
	}
	delete(b.data, k)
	return nil
}

func testBinding() identity.Binding {
	return identity.Binding{Namespace: "tinyrill/v1", Customer: "test", Protocol: "atrust", Server: "vpn.example:443", Domain: "domain", Username: "user", Ref: "login"}
}

func TestCredentialBindingAndStoreFailures(t *testing.T) {
	b := &memoryBackend{data: map[string][]byte{}}
	s := newStore(testBinding(), b)
	if err := s.Set(t.Context(), "sample-password"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get(t.Context()); err != nil || got != "sample-password" {
		t.Fatalf("get: %v", err)
	}
	for _, field := range []string{"protocol", "server", "domain", "username"} {
		binding := testBinding()
		switch field {
		case "protocol":
			binding.Protocol = "easyconnect"
		case "server":
			binding.Server = "other.example:443"
		case "domain":
			binding.Domain = "other"
		case "username":
			binding.Username = "other"
		}
		if _, err := newStore(binding, b).Get(t.Context()); !errors.Is(err, ErrBinding) {
			t.Fatalf("%s binding: %v", field, err)
		}
	}
	b.failure = ErrLocked
	if err := s.Set(t.Context(), "replacement"); !errors.Is(err, ErrLocked) {
		t.Fatal("storage failure hidden")
	}
	b.failure = nil
	if got, _ := s.Get(t.Context()); got != "sample-password" {
		t.Fatal("failed save replaced old credential")
	}
	if err := s.Delete(t.Context()); err != nil {
		t.Fatal(err)
	}
	if status, err := s.Status(t.Context()); err != nil || status != "absent" {
		t.Fatal("deleted credential remains")
	}
}

func TestAgeVaultAuthenticationAndAtomicUpdates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "password.age")
	prompt := func(context.Context, string) (string, error) { return "separate-unlock-phrase", nil }
	b := &ageBackend{path: path, prompt: prompt, workFactor: 10}
	s := newStore(testBinding(), b)
	defer s.Close()
	if err := s.Set(t.Context(), "vpn-password-marker"); err != nil {
		t.Fatal(err)
	}
	if err := securefile.Check(path); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(original, []byte("vpn-password-marker")) || bytes.Contains(original, []byte("separate-unlock-phrase")) {
		t.Fatal("plaintext written to vault")
	}
	if got, err := s.Get(t.Context()); err != nil || got != "vpn-password-marker" {
		t.Fatalf("decrypt: %v", err)
	}
	wrong := newStore(testBinding(), &ageBackend{path: path, prompt: func(context.Context, string) (string, error) { return "wrong", nil }, workFactor: 10})
	defer wrong.Close()
	if err := wrong.Set(t.Context(), "do-not-overwrite"); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("wrong unlock: %v", err)
	}
	unchanged, _ := os.ReadFile(path)
	if !bytes.Equal(unchanged, original) {
		t.Fatal("wrong unlock destroyed vault")
	}
	for _, corrupt := range [][]byte{original[:len(original)-1], bytes.Replace(original, []byte(" 10\n"), []byte(" 30\n"), 1)} {
		if err := securefile.Write(path, corrupt); err != nil {
			t.Fatal(err)
		}
		if got, err := s.Get(t.Context()); !errors.Is(err, ErrDecrypt) || got != "" {
			t.Fatal("unauthenticated or over-cost plaintext escaped")
		}
	}
}

// Opt-in native integration uses a unique synthetic entry and always deletes it.
func TestNativeCredentialRoundTrip(t *testing.T) {
	if os.Getenv("TINYRILL_TEST_NATIVE_CREDENTIALS") != "1" {
		t.Skip("native credential service integration is opt-in")
	}
	binding := testBinding()
	binding.Customer = "integration-" + rand.Text()
	s := newStore(binding, systemBackend{})
	t.Cleanup(func() {
		if err := s.Delete(context.Background()); err != nil {
			t.Errorf("delete synthetic test credential: %v", err)
		}
	})
	if err := s.Set(t.Context(), "synthetic-integration-secret"); err != nil {
		t.Fatal(err)
	}
	if value, err := s.Get(t.Context()); err != nil || value != "synthetic-integration-secret" {
		t.Fatalf("native read: %v", err)
	}
	if err := s.Set(t.Context(), "updated-synthetic-secret"); err != nil {
		t.Fatal(err)
	}
	if value, err := s.Get(t.Context()); err != nil || value != "updated-synthetic-secret" {
		t.Fatalf("native update: %v", err)
	}
	if err := s.Delete(t.Context()); err != nil {
		t.Fatal(err)
	}
	if status, err := s.Status(t.Context()); err != nil || status != "absent" {
		t.Fatalf("native deletion status: %v", err)
	}
}
