package https

import (
	"context"
	"errors"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func newRequestDNSMsg() *dns.Msg {
	return &dns.Msg{Question: []dns.Question{
		{
			Name:   "example.com.",
			Qtype:  dns.TypeA,
			Qclass: dns.ClassINET,
		},
	}}
}

func TestHTTPS(t *testing.T) {
	dnsMsg := newRequestDNSMsg()
	dnsdata, err := dnsMsg.Pack()
	require.NoError(t, err)
	dnsClient := &mockDNSClient{reqBody: dnsdata, t: t}
	h := newHTTPS(".", dnsClient)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	status, err := h.ServeDNS(context.Background(), rec, dnsMsg)
	require.NoError(t, err)
	require.Equal(t, dns.RcodeSuccess, status)
	require.Equal(t, 1, dnsClient.callCount, "dnsClient call count is wrong")
	require.Equal(t, newExpectedDNSMsg(), rec.Msg)
}

type mockDNSClientFunc func(ctx context.Context, dnsreq []byte) (*dns.Msg, error)

func (f mockDNSClientFunc) Query(ctx context.Context, dnsreq []byte) (*dns.Msg, error) {
	return f(ctx, dnsreq)
}

type mockDNSResponseWriter struct {
	dns.ResponseWriter
	writeFunc func(*dns.Msg) error
}

func (w *mockDNSResponseWriter) WriteMsg(msg *dns.Msg) error {
	return w.writeFunc(msg)
}

func TestHTTPSMsgPackError(t *testing.T) {
	dnsMsg := &dns.Msg{MsgHdr: dns.MsgHdr{Rcode: 0xFFFFF}}
	dnsClient := mockDNSClientFunc(func(_ context.Context, _ []byte) (result *dns.Msg, err error) {
		t.Fatal("dns client must not be called")
		return
	})
	h := newHTTPS(".", dnsClient)
	w := &mockDNSResponseWriter{
		ResponseWriter: &test.ResponseWriter{},
		writeFunc: func(*dns.Msg) (err error) {
			t.Fatal("dns response writer must not be called")
			return
		},
	}

	status, err := h.ServeDNS(context.Background(), w, dnsMsg)
	require.Error(t, err)
	require.Equal(t, dns.RcodeServerFailure, status)
}

func TestHTTPSDNSClientError(t *testing.T) {
	dnsMsg := newRequestDNSMsg()
	dnsClient := mockDNSClientFunc(func(_ context.Context, _ []byte) (*dns.Msg, error) {
		return newExpectedDNSMsg(), errors.New("dns client error")
	})
	h := newHTTPS(".", dnsClient)
	w := &mockDNSResponseWriter{
		ResponseWriter: &test.ResponseWriter{},
		writeFunc: func(*dns.Msg) (err error) {
			t.Fatal("dns response writer must not be called")
			return
		},
	}

	status, err := h.ServeDNS(context.Background(), w, dnsMsg)
	require.Error(t, err)
	require.Equal(t, dns.RcodeServerFailure, status)
}

func TestHTTPSResponseWriterError(t *testing.T) {
	dnsMsg := newRequestDNSMsg()
	dnsClient := mockDNSClientFunc(func(_ context.Context, _ []byte) (*dns.Msg, error) {
		return newExpectedDNSMsg(), nil
	})
	h := newHTTPS(".", dnsClient)
	w := &mockDNSResponseWriter{
		ResponseWriter: &test.ResponseWriter{},
		writeFunc: func(*dns.Msg) (err error) {
			return errors.New("response writer error")
		},
	}

	_, err := h.ServeDNS(context.Background(), w, dnsMsg)
	require.Error(t, err)
}

func TestHTTPSDNSResponseStateNotMatch(t *testing.T) {
	dnsMsg := newRequestDNSMsg()
	dnsClient := mockDNSClientFunc(func(_ context.Context, _ []byte) (*dns.Msg, error) {
		result := newExpectedDNSMsg()
		result.Question[0].Name = "other.domain."
		return result, nil
	})
	h := newHTTPS(".", dnsClient)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	_, err := h.ServeDNS(context.Background(), rec, dnsMsg)
	require.NoError(t, err)
	require.Equal(t, dns.RcodeFormatError, rec.Rcode)
}
