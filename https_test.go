package https

import (
	"context"
	"errors"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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
	t.Parallel()

	dnsMsg := newRequestDNSMsg()
	dnsdata, err := dnsMsg.Pack()
	require.NoError(t, err)
	dnsClient := NewMockdnsClient(gomock.NewController(t))
	dnsClient.EXPECT().Query(gomock.Any(), dnsdata).Return(newExpectedDNSMsg(), nil)
	h := newHTTPS(".", dnsClient)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	status, err := h.ServeDNS(context.Background(), rec, dnsMsg)
	require.NoError(t, err)
	require.Equal(t, dns.RcodeSuccess, status)
	require.Equal(t, newExpectedDNSMsg(), rec.Msg)
}

func TestHTTPSMsgPackError(t *testing.T) {
	t.Parallel()

	dnsMsg := &dns.Msg{MsgHdr: dns.MsgHdr{Rcode: 0xFFFFF}}
	ctrl := gomock.NewController(t)
	dnsClient := NewMockdnsClient(ctrl)
	h := newHTTPS(".", dnsClient)
	w := NewMockResponseWriter(ctrl)

	status, err := h.ServeDNS(context.Background(), w, dnsMsg)
	require.Error(t, err)
	require.Equal(t, dns.RcodeServerFailure, status)
}

func TestHTTPSDNSClientError(t *testing.T) {
	t.Parallel()

	dnsMsg := newRequestDNSMsg()
	ctrl := gomock.NewController(t)
	dnsClient := NewMockdnsClient(ctrl)
	dnsClient.EXPECT().Query(gomock.Any(), gomock.Any()).Return(newExpectedDNSMsg(), errors.New("dns client error"))
	h := newHTTPS(".", dnsClient)
	w := NewMockResponseWriter(ctrl)

	status, err := h.ServeDNS(context.Background(), w, dnsMsg)
	require.Error(t, err)
	require.Equal(t, dns.RcodeServerFailure, status)
}

func TestHTTPSResponseWriterError(t *testing.T) {
	t.Parallel()

	dnsMsg := newRequestDNSMsg()
	ctrl := gomock.NewController(t)
	dnsClient := NewMockdnsClient(ctrl)
	dnsClient.EXPECT().Query(gomock.Any(), gomock.Any()).Return(newExpectedDNSMsg(), nil)
	h := newHTTPS(".", dnsClient)
	w := NewMockResponseWriter(ctrl)
	w.EXPECT().WriteMsg(gomock.Any()).Return(errors.New("response writer error"))

	_, err := h.ServeDNS(context.Background(), w, dnsMsg)
	require.Error(t, err)
}

func TestHTTPSDNSResponseStateNotMatch(t *testing.T) {
	t.Parallel()

	dnsMsg := newRequestDNSMsg()
	dnsClient := NewMockdnsClient(gomock.NewController(t))
	dnsClient.EXPECT().Query(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, []byte) (*dns.Msg, error) {
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
