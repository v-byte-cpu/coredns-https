//go:build windows

package https

import (
	"crypto/tls"
	"fmt"

	"golang.org/x/sys/windows"

	"github.com/coredns/caddy"
	"github.com/google/certtostore"
)

func parseTLSCertStore(c *caddy.Controller, conf *httpsConfig) (err error) {
	if !c.NextArg() {
		return c.ArgErr()
	}
	store := c.Val()

	if !c.NextArg() {
		return c.ArgErr()
	}
	provider := c.Val()

	if !c.NextArg() {
		return c.ArgErr()
	}
	container := c.Val()

	if !c.NextArg() {
		return c.ArgErr()
	}
	issuer := c.Val()

	if !c.NextArg() {
		return c.ArgErr()
	}
	intermediateIssuer := c.Val()

	var providerArg string
	if provider == "software" {
		providerArg = certtostore.ProviderMSSoftware
	} else if provider == "platform" {
		providerArg = certtostore.ProviderMSPlatform
	} else {
		return fmt.Errorf("Unknown cert store provider %s", provider)
	}

	var certStore certtostore.WinCertStorage = nil
	var certContext *windows.CertContext = nil

	shutdownHandler := func() error {
		if certContext != nil {
			err := certtostore.FreeCertContext(certContext)
			if err != nil {
				return err
			}
			certContext = nil
		}
		if certStore != nil {
			err = certStore.Close()
			if err != nil {
				return err
			}
			certStore = nil
		}
		return nil
	}
	c.OnShutdown(shutdownHandler)

	if store == "user" {
		certStore, err = certtostore.OpenWinCertStoreCurrentUser(providerArg, container, []string{issuer}, []string{intermediateIssuer}, false)
	} else if store == "system" {
		certStore, err = certtostore.OpenWinCertStore(providerArg, container, []string{issuer}, []string{intermediateIssuer}, false)
	} else {
		return fmt.Errorf("Unknown cert store type \"%s\"", store)
	}

	if err != nil {
		return fmt.Errorf("Failed to open certificate store: %w", err)
	}

	cert, certContext, err := certStore.CertWithContext()
	if err != nil {
		return fmt.Errorf("Failed to retrieve certificate: %w", err)
	}

	if len(cert.Raw) == 0 {
		return fmt.Errorf("Empty certificate retrieved")
	}

	key, err := certStore.CertKey(certContext)
	if err != nil {
		return fmt.Errorf("Failed to retrieve certificate key: %w", err)
	}

	tlsCert := tls.Certificate{
		Certificate: [][]byte{cert.Raw},
		PrivateKey:  key,
		Leaf:        cert,
	}
	log.Infof("Using client certificate with CN: %s", tlsCert.Leaf.Subject.CommonName)

	if conf.tlsConfig == nil {
		conf.tlsConfig = new(tls.Config)
	}
	conf.tlsConfig.Certificates = append(conf.tlsConfig.Certificates, tlsCert)
	return nil
}
