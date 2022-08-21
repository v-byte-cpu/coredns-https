package https

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	pkgtls "github.com/coredns/coredns/plugin/pkg/tls"
)

const maxUpstreams = 15

func init() { plugin.Register("https", setup) }

func setup(c *caddy.Controller) error {
	conf, err := parseConfig(c)
	if err != nil {
		return plugin.Error("https", err)
	}

	dnsClient := setupDNSClient(conf)
	h := newHTTPS(conf.from, dnsClient, withExcept(conf.except))
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		h.Next = next
		return h
	})

	return nil
}

func setupDNSClient(conf *httpsConfig) dnsClient {
	tr := &http.Transport{
		TLSClientConfig:   conf.tlsConfig,
		ForceAttemptHTTP2: true,
	}
	httpClient := &http.Client{
		Transport: tr,
	}

	clients := make([]dnsClient, len(conf.toURLs))
	for i, toURL := range conf.toURLs {
		clients[i] = newMetricDNSClient(newDoHDNSClient(httpClient, toURL), toURL)
	}

	var opts []lbDNSClientOption
	if conf.policy != nil {
		opts = append(opts, withLbPolicy(conf.policy))
	}

	// TODO request timeout, max_fail options
	return newLoadBalanceDNSClient(clients, opts...)
}

type httpsConfig struct {
	from          string
	toURLs        []string
	except        []string
	tlsConfig     *tls.Config
	tlsServerName string
	policy        policy
}

func parseConfig(c *caddy.Controller) (conf *httpsConfig, err error) {
	conf = &httpsConfig{}
	if !c.Next() {
		return conf, c.ArgErr()
	}
	if !c.Args(&conf.from) {
		return conf, c.ArgErr()
	}
	conf.from, err = parseHost(conf.from)
	if err != nil {
		return conf, err
	}

	toURLs := c.RemainingArgs()
	if len(toURLs) == 0 {
		return conf, c.ArgErr()
	}
	if len(toURLs) > maxUpstreams {
		return conf, fmt.Errorf("more than %d TOs configured: %d", maxUpstreams, len(toURLs))
	}
	conf.toURLs = make([]string, 0, len(toURLs))
	for _, to := range toURLs {
		toURL := "https://" + to
		if _, err := url.ParseRequestURI(toURL); err != nil {
			return conf, err
		}
		conf.toURLs = append(conf.toURLs, toURL)
	}

	for c.NextBlock() {
		if err := parseBlock(c, conf); err != nil {
			return conf, err
		}
	}

	if conf.tlsServerName != "" {
		if conf.tlsConfig == nil {
			conf.tlsConfig = new(tls.Config)
		}
		conf.tlsConfig.ServerName = conf.tlsServerName
	}

	return conf, nil
}

func parseBlock(c *caddy.Controller, conf *httpsConfig) (err error) {
	f, ok := parseBlockMap[c.Val()]
	if !ok {
		return c.Errf("unknown property '%s'", c.Val())
	}
	return f(c, conf)
}

type parseBlockFunc func(*caddy.Controller, *httpsConfig) error

var parseBlockMap = map[string]parseBlockFunc{
	"except":         parseExcept,
	"tls":            parseTLS,
	"tls_servername": parseTLSServerName,
	"policy":         parsePolicy,
}

func parseExcept(c *caddy.Controller, conf *httpsConfig) (err error) {
	except := c.RemainingArgs()
	if len(except) == 0 {
		return c.ArgErr()
	}
	for i := 0; i < len(except); i++ {
		if except[i], err = parseHost(except[i]); err != nil {
			return
		}
	}
	conf.except = except
	return
}

func parseHost(hostAddr string) (string, error) {
	hosts := plugin.Host(hostAddr).NormalizeExact()
	if len(hosts) == 0 {
		return "", fmt.Errorf("unable to normalize '%s'", hostAddr)
	}
	return plugin.Name(hosts[0]).Normalize(), nil
}

func parseTLS(c *caddy.Controller, conf *httpsConfig) error {
	args := c.RemainingArgs()
	tlsConfig, err := pkgtls.NewTLSConfigFromArgs(args...)
	if err != nil {
		return err
	}
	conf.tlsConfig = tlsConfig
	return nil
}

func parseTLSServerName(c *caddy.Controller, conf *httpsConfig) error {
	args := c.RemainingArgs()
	if len(args) != 1 {
		return c.ArgErr()
	}
	conf.tlsServerName = args[0]
	return nil
}

func parsePolicy(c *caddy.Controller, conf *httpsConfig) error {
	args := c.RemainingArgs()
	if len(args) != 1 {
		return c.ArgErr()
	}
	switch args[0] {
	case "random":
		conf.policy = newRandomPolicy()
	case "round_robin":
		conf.policy = newRoundRobinPolicy()
	case "sequential":
		conf.policy = newSequentialPolicy()
	default:
		return c.Errf("unknown policy '%s'", args[0])
	}
	return nil
}
