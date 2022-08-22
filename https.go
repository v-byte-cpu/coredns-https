// Package https implements a plugin that performs DNS-over-HTTPS proxying.
//
// See: RFC 8484 (https://tools.ietf.org/html/rfc8484)
package https

import (
	"context"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/debug"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

// HTTPS represents a plugin instance that can proxy requests to another (DNS) server via DoH protocol.
// It has a list of proxies each representing one upstream proxy
type HTTPS struct {
	from   string
	except []string
	client dnsClient
	Next   plugin.Handler
}

type httpsOption func(h *HTTPS)

func withExcept(except []string) httpsOption {
	return func(h *HTTPS) {
		h.except = except
	}
}

// newHTTPS returns a new HTTPS.
func newHTTPS(from string, client dnsClient, opts ...httpsOption) *HTTPS {
	h := &HTTPS{from: from, client: client}
	for _, o := range opts {
		o(h)
	}
	return h
}

// Assert that HTTPS struct conforms to the plugin.Handler interface
var _ plugin.Handler = (*HTTPS)(nil)

// Name implements plugin.Handler.
func (*HTTPS) Name() string { return "https" }

// ServeDNS implements plugin.Handler.
func (h *HTTPS) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (status int, err error) {
	state := request.Request{W: w, Req: r}
	if !h.match(state) {
		return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
	}

	dnsreq, err := r.Pack()
	if err != nil {
		return dns.RcodeServerFailure, err
	}
	result, err := h.client.Query(ctx, dnsreq)
	if err != nil {
		return dns.RcodeServerFailure, err
	}

	// Check if the reply is correct; if not return FormErr.
	// maybe useful to extract this logic from forward, grpc and https plugins to common place
	if !state.Match(result) {
		debug.Hexdumpf(result, "Wrong reply for id: %d, %s %d", result.Id, state.QName(), state.QType())

		formerr := new(dns.Msg)
		formerr.SetRcode(state.Req, dns.RcodeFormatError)
		err = w.WriteMsg(formerr)
		return
	}

	err = w.WriteMsg(result)
	return
}

// TODO extract this logic from forward, grpc and https plugins to common place
func (h *HTTPS) match(state request.Request) bool {
	if !plugin.Name(h.from).Matches(state.Name()) || !h.isAllowedDomain(state.Name()) {
		return false
	}

	return true
}

func (h *HTTPS) isAllowedDomain(name string) bool {
	if dns.Name(name) == dns.Name(h.from) {
		return true
	}

	for _, ignore := range h.except {
		if plugin.Name(ignore).Matches(name) {
			return false
		}
	}
	return true
}
