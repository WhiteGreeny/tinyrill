package credentials

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"filippo.io/age"
	"github.com/mythologyli/zju-connect/internal/securefile"
)

type ageBackend struct {
	path       string
	prompt     Prompt
	passphrase []byte
	workFactor int
}

func (b *ageBackend) unlock(ctx context.Context, create bool) error {
	if len(b.passphrase) > 0 {
		return nil
	}
	if b.prompt == nil {
		return ErrLocked
	}
	pass, err := b.prompt(ctx, "Age vault passphrase:")
	if err != nil {
		return err
	}
	if len(pass) < 1 || len(pass) > 4096 {
		return ErrLocked
	}
	if create {
		confirm, err := b.prompt(ctx, "Confirm age vault passphrase:")
		if err != nil {
			return err
		}
		if pass != confirm {
			return errors.New("age vault passphrases do not match")
		}
	}
	b.passphrase = []byte(pass)
	return nil
}

func (b *ageBackend) Get(ctx context.Context, _ string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := securefile.Check(b.path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	ciphertext, err := securefile.ReadLimited(b.path, 1<<20)
	if err != nil {
		return nil, err
	}
	if err = b.unlock(ctx, false); err != nil {
		return nil, err
	}
	id, err := age.NewScryptIdentity(string(b.passphrase))
	if err != nil {
		return nil, ErrDecrypt
	}
	id.SetMaxWorkFactor(18)
	reader, err := age.Decrypt(bytes.NewReader(ciphertext), id)
	if err != nil {
		b.Close()
		return nil, ErrDecrypt
	}
	// Read to EOF before releasing plaintext: age authenticates the final chunk.
	data, err := io.ReadAll(io.LimitReader(reader, maxRecord+1))
	if err != nil || len(data) > maxRecord {
		clear(data)
		b.Close()
		return nil, ErrDecrypt
	}
	if err := ctx.Err(); err != nil {
		clear(data)
		return nil, err
	}
	return data, nil
}

func (b *ageBackend) Set(ctx context.Context, key string, data []byte) error {
	if len(data) > maxRecord {
		return errors.New("credential record exceeds size limit")
	}
	// An existing vault must authenticate before replacement; mistyped unlock
	// phrases must not destroy the only readable copy.
	old, err := b.Get(ctx, key)
	clear(old)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if err = b.unlock(ctx, errors.Is(err, ErrNotFound)); err != nil {
		return err
	}
	recipient, err := age.NewScryptRecipient(string(b.passphrase))
	if err != nil {
		return err
	}
	recipient.SetWorkFactor(b.workFactor)
	var ciphertext bytes.Buffer
	w, err := age.Encrypt(&ciphertext, recipient)
	if err != nil {
		return err
	}
	if _, err = w.Write(data); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Only create the vault's parent if absent; never change permissions on a
	// user's existing home or shared configuration directory.
	if _, err := os.Stat(filepath.Dir(b.path)); errors.Is(err, os.ErrNotExist) {
		if err := securefile.MakeDir(filepath.Dir(b.path)); err != nil {
			return err
		}
	}
	return securefile.Write(b.path, ciphertext.Bytes())
}
func (b *ageBackend) Delete(ctx context.Context, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := os.Remove(b.path)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	return err
}
func (b *ageBackend) Close() { clear(b.passphrase); b.passphrase = nil }
