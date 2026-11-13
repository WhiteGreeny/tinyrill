package keylog

import (
	"github.com/mythologyli/zju-connect/internal/securefile"
	"io"
)

func Open(path string) (io.WriteCloser, error) {
	if path == "" {
		return nil, nil
	}
	return securefile.Create(path)
}
