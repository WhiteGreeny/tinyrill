package instance

import (
	"github.com/mythologyli/zju-connect/configs"
	"github.com/mythologyli/zju-connect/service"
)

type serviceListeners = service.Listeners

func reserve(p configs.Profile) (*owner, error) {
	listeners, err := service.Reserve(p.Listen.HTTP, p.Listen.SOCKS5)
	if err != nil {
		return nil, err
	}
	return &owner{proxies: listeners}, nil
}
