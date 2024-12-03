//go:build !windows

package https

import (
	"errors"

	"github.com/coredns/caddy"
)

func parseTLSCertStore(c *caddy.Controller, conf *httpsConfig) (err error) {
	log.Error("Client cert from cert store only supported on Windows")
	return errors.New("client cert from cert store only supported on Windows")
}
