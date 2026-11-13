// Package credentials stores a bound VPN password through the OS secret store
// or an explicitly selected age vault. It never falls back to plaintext.
package credentials

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mythologyli/zju-connect/configs"
	"github.com/mythologyli/zju-connect/internal/identity"
)

var (
	ErrNotFound    = errors.New("saved password not found")
	ErrUnavailable = errors.New("system credential service is unavailable")
	ErrLocked      = errors.New("credential service is locked or access was denied")
	ErrBinding     = errors.New("saved password belongs to a different server or account; use credentials set to explicitly replace it")
	ErrDecrypt     = errors.New("age vault authentication failed, or its size/work factor is unsupported")
)

const maxRecord = 32 << 10

type Backend interface {
	Get(context.Context, string) ([]byte, error)
	Set(context.Context, string, []byte) error
	Delete(context.Context, string) error
}
type Prompt func(context.Context, string) (string, error)
type Store struct {
	backend Backend
	binding identity.Binding
	key     string
}
type record struct {
	Binding  identity.Binding `json:"binding"`
	Password string           `json:"password"`
}

func New(p configs.Profile, prompt Prompt) (*Store, error) {
	var b Backend
	switch p.Credentials.Backend {
	case "auto":
		b = systemBackend{}
	case "age-file":
		b = &ageBackend{path: p.Credentials.VaultFile, prompt: prompt, workFactor: 18}
	default:
		return nil, errors.New("unsupported credential backend")
	}
	return newStore(identity.ForProfile(p), b), nil
}

func newStore(binding identity.Binding, b Backend) *Store {
	keyData, _ := json.Marshal([]string{binding.Namespace, binding.Customer, binding.Ref})
	sum := sha256.Sum256(keyData)
	return &Store{backend: b, binding: binding, key: "tinyrill/v1/" + hex.EncodeToString(sum[:])}
}

func (s *Store) Get(ctx context.Context) (string, error) {
	data, err := s.backend.Get(ctx, s.key)
	if err != nil {
		return "", err
	}
	defer clear(data)
	var r record
	if len(data) > maxRecord || json.Unmarshal(data, &r) != nil {
		return "", errors.New("invalid saved credential record")
	}
	if r.Binding != s.binding {
		return "", ErrBinding
	}
	if r.Password == "" {
		return "", errors.New("empty saved password")
	}
	return r.Password, nil
}

func (s *Store) Set(ctx context.Context, password string) error {
	if password == "" || len(password) > 4096 {
		return errors.New("password must contain 1–4096 bytes")
	}
	data, err := json.Marshal(record{s.binding, password})
	if err != nil {
		return err
	}
	defer clear(data)
	return s.backend.Set(ctx, s.key, data)
}
func (s *Store) Delete(ctx context.Context) error {
	err := s.backend.Delete(ctx, s.key)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
func (s *Store) Status(ctx context.Context) (string, error) {
	_, err := s.Get(ctx)
	if errors.Is(err, ErrNotFound) {
		return "absent", nil
	}
	if err != nil {
		return "unavailable", err
	}
	return "present", nil
}
func (s *Store) Close() {
	if b, ok := s.backend.(interface{ Close() }); ok {
		b.Close()
	}
}

func nativeError(operation string, err error) error {
	return fmt.Errorf("credential %s: %w", operation, err)
}
