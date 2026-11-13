//go:build (!windows && !linux && !darwin) || (darwin && !cgo)

package credentials

import "context"

type systemBackend struct{}

func (systemBackend) Get(context.Context, string) ([]byte, error) { return nil, ErrUnavailable }
func (systemBackend) Set(context.Context, string, []byte) error   { return ErrUnavailable }
func (systemBackend) Delete(context.Context, string) error        { return ErrUnavailable }
