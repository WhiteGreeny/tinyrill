// Package state owns one customer's private state directory and process lock.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/mythologyli/zju-connect/internal/identity"
	"github.com/mythologyli/zju-connect/internal/securefile"
)

var ErrLocked = errors.New("state directory is in use by another instance")
var ErrBinding = errors.New("saved session belongs to another identity; use session clear before changing it")

type Directory struct {
	path string
	lock *os.File
	once sync.Once
}

func Acquire(path string) (*Directory, error) {
	if err := securefile.MakeDir(path); err != nil {
		return nil, fmt.Errorf("prepare state directory: %w", err)
	}
	f, err := securefile.OpenLock(filepath.Join(path, "instance.lock"))
	if err != nil {
		return nil, err
	}
	if err := tryLock(f); err != nil {
		f.Close()
		return nil, err
	}
	return &Directory{path: path, lock: f}, nil
}

func (d *Directory) Close() error {
	var err error
	d.once.Do(func() { err = d.lock.Close() })
	return err
}

type envelope struct {
	Binding identity.Binding `json:"binding"`
	Data    json.RawMessage  `json:"data"`
}

func (d *Directory) LoadSession(binding identity.Binding) ([]byte, error) {
	path := filepath.Join(d.path, "session.json")
	if err := securefile.Check(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("session permissions: %w", err)
	}
	data, err := securefile.ReadLimited(path, 4<<20)
	if err != nil {
		return nil, err
	}
	var stored envelope
	if json.Unmarshal(data, &stored) != nil {
		return nil, errors.New("invalid session state")
	}
	if stored.Binding != binding {
		return nil, ErrBinding
	}
	return stored.Data, nil
}

func (d *Directory) SaveSession(binding identity.Binding, data []byte) error {
	if len(data) > 4<<20 || !json.Valid(data) {
		return errors.New("invalid or oversized session state")
	}
	encoded, err := json.Marshal(envelope{binding, data})
	if err != nil {
		return err
	}
	return securefile.Write(filepath.Join(d.path, "session.json"), encoded)
}

func (d *Directory) ClearSession() error {
	err := os.Remove(filepath.Join(d.path, "session.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (d *Directory) SaveStatus(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return securefile.Write(filepath.Join(d.path, "status.json"), data)
}
