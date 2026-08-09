package https

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const upstreamURL = "https://example.com/dns-query"

func newExpectedDNSMsg() *dns.Msg {
	return &dns.Msg{
		MsgHdr: dns.MsgHdr{Response: true},
		Question: []dns.Question{
			{
				Name:   "example.com.",
				Qtype:  dns.TypeA,
				Qclass: dns.ClassINET,
			},
		},
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30},
				A:   net.IPv4(1, 1, 1, 1),
			},
		},
	}
}

func packMsg(t *testing.T, msg *dns.Msg) []byte {
	t.Helper()
	data, err := msg.Pack()
	require.NoError(t, err)
	return data
}

func TestDNSClient(t *testing.T) {
	t.Parallel()

	expectedMsg := newExpectedDNSMsg()
	httpClient := NewMockhttpRequestDoer(gomock.NewController(t))
	httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (resp *http.Response, err error) {
		acceptHdrs := req.Header["Accept"]
		require.NotEmpty(t, acceptHdrs, "Accept header is empty")
		require.Equal(t, dnsMessageMimeType, acceptHdrs[0], "invalid accept header")

		contentTypeHdrs := req.Header["Content-Type"]
		require.NotEmpty(t, contentTypeHdrs, "Content-Type header is empty")
		require.Equal(t, dnsMessageMimeType, contentTypeHdrs[0], "invalid Content-Type header")

		require.Equal(t, upstreamURL, req.URL.String(), "invalid request URL")
		require.Equal(t, "POST", req.Method, "invalid request method")

		buf, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Equal(t, []byte("abc"), buf, "invalid request body")

		resp = &http.Response{
			Body:       io.NopCloser(bytes.NewReader(packMsg(t, expectedMsg))),
			StatusCode: http.StatusOK,
		}
		return
	})
	dnsClient := newDoHDNSClient(httpClient, upstreamURL)

	result, err := dnsClient.Query(context.Background(), []byte("abc"))
	require.NoError(t, err)
	require.True(t, result.Response)
	require.Len(t, result.Answer, len(expectedMsg.Answer))
	require.Equal(t, expectedMsg.Answer[0].String(), result.Answer[0].String())
}

func TestDNSClientNewRequestError(t *testing.T) {
	t.Parallel()

	invalidURL := "https://example.com/\t\n"
	httpClient := NewMockhttpRequestDoer(gomock.NewController(t))
	dnsClient := newDoHDNSClient(httpClient, invalidURL)

	_, err := dnsClient.Query(context.Background(), []byte("abc"))
	require.Error(t, err)
}

func TestDNSClientHttpClientError(t *testing.T) {
	t.Parallel()

	httpClient := NewMockhttpRequestDoer(gomock.NewController(t))
	httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(*http.Request) (resp *http.Response, err error) {
		err = errors.New("http error")
		resp = &http.Response{
			Body:       io.NopCloser(bytes.NewReader(packMsg(t, newExpectedDNSMsg()))),
			StatusCode: http.StatusOK,
		}
		return
	})
	dnsClient := newDoHDNSClient(httpClient, upstreamURL)

	_, err := dnsClient.Query(context.Background(), []byte("abc"))
	require.Error(t, err)
}

func TestDNSClientHttpStatusError(t *testing.T) {
	t.Parallel()

	httpClient := NewMockhttpRequestDoer(gomock.NewController(t))
	httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(*http.Request) (resp *http.Response, err error) {
		resp = &http.Response{
			Body:       io.NopCloser(bytes.NewReader(packMsg(t, newExpectedDNSMsg()))),
			StatusCode: http.StatusInternalServerError,
		}
		return
	})
	dnsClient := newDoHDNSClient(httpClient, upstreamURL)

	_, err := dnsClient.Query(context.Background(), []byte("abc"))
	require.Error(t, err)
}

type errReader struct {
	delegate io.Reader
	err      error
}

// return err instead of io.EOF after the last piece of data is read
func (r *errReader) Read(p []byte) (n int, err error) {
	n, err = r.delegate.Read(p)
	if err == io.EOF {
		err = r.err
	}
	return
}

func TestDNSClientResponseReadError(t *testing.T) {
	t.Parallel()

	httpClient := NewMockhttpRequestDoer(gomock.NewController(t))
	httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(*http.Request) (resp *http.Response, err error) {
		reader := &errReader{
			delegate: bytes.NewReader(packMsg(t, newExpectedDNSMsg())),
			err:      errors.New("io error"),
		}
		resp = &http.Response{
			Body:       io.NopCloser(reader),
			StatusCode: http.StatusOK,
		}
		return
	})
	dnsClient := newDoHDNSClient(httpClient, upstreamURL)

	_, err := dnsClient.Query(context.Background(), []byte("abc"))
	require.Error(t, err)
}

func TestDNSClientMsgUnpackError(t *testing.T) {
	t.Parallel()

	httpClient := NewMockhttpRequestDoer(gomock.NewController(t))
	httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(*http.Request) (resp *http.Response, err error) {
		resp = &http.Response{
			Body:       io.NopCloser(strings.NewReader("def")),
			StatusCode: http.StatusOK,
		}
		return
	})
	dnsClient := newDoHDNSClient(httpClient, upstreamURL)

	_, err := dnsClient.Query(context.Background(), []byte("abc"))
	require.Error(t, err)
}

func TestDNSClientLargeResponseError(t *testing.T) {
	t.Parallel()

	// create large buffer with the correct DNS message at the beginning
	var buf bytes.Buffer
	dnsMsg := packMsg(t, newExpectedDNSMsg())
	buf.Write(dnsMsg)
	buf.WriteString(strings.Repeat("a", maxDNSMessageSize))

	httpClient := NewMockhttpRequestDoer(gomock.NewController(t))
	httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(*http.Request) (resp *http.Response, err error) {
		resp = &http.Response{
			Body:       io.NopCloser(&buf),
			StatusCode: http.StatusOK,
		}
		return
	})
	dnsClient := newDoHDNSClient(httpClient, upstreamURL)

	_, err := dnsClient.Query(context.Background(), []byte("abc"))
	require.Error(t, err)
}

func TestLoadBalanceDNSClient(t *testing.T) {
	t.Parallel()

	client1 := NewMockdnsClient(gomock.NewController(t))
	client1.EXPECT().Query(gomock.Any(), []byte("abc")).Return(newExpectedDNSMsg(), nil)
	clients := []dnsClient{client1}
	lbClient := newLoadBalanceDNSClient(clients)

	result, err := lbClient.Query(context.Background(), []byte("abc"))
	require.NoError(t, err)
	require.Equal(t, newExpectedDNSMsg(), result)
}

func TestLoadBalanceDNSClientError(t *testing.T) {
	t.Parallel()

	client1 := NewMockdnsClient(gomock.NewController(t))
	client1.EXPECT().Query(gomock.Any(), []byte("abc")).Return(newExpectedDNSMsg(), errors.New("client error"))
	clients := []dnsClient{client1}
	lbClient := newLoadBalanceDNSClient(clients)

	_, err := lbClient.Query(context.Background(), []byte("abc"))
	require.Error(t, err)
}

func TestLoadBalanceDNSClientRetry(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client1 := NewMockdnsClient(ctrl)
	client2 := NewMockdnsClient(ctrl)
	gomock.InOrder(
		client1.EXPECT().Query(gomock.Any(), []byte("abc")).Return(newExpectedDNSMsg(), errors.New("client error")),
		client2.EXPECT().Query(gomock.Any(), []byte("abc")).Return(newExpectedDNSMsg(), nil),
	)
	clients := []dnsClient{client1, client2}
	lbClient := newLoadBalanceDNSClient(clients, withLbPolicy(newSequentialPolicy()))

	result, err := lbClient.Query(context.Background(), []byte("abc"))
	require.NoError(t, err)
	require.Equal(t, newExpectedDNSMsg(), result)
}

func TestLoadBalanceDNSClientPolicy(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client1 := NewMockdnsClient(ctrl)
	client2 := NewMockdnsClient(ctrl)
	gomock.InOrder(
		client1.EXPECT().Query(gomock.Any(), []byte("abc")).Return(newExpectedDNSMsg(), nil),
		client2.EXPECT().Query(gomock.Any(), []byte("abc")).Return(newExpectedDNSMsg(), nil),
	)
	clients := []dnsClient{client1, client2}
	lbClient := newLoadBalanceDNSClient(clients, withLbPolicy(newRoundRobinPolicy()))

	result, err := lbClient.Query(context.Background(), []byte("abc"))
	require.NoError(t, err)
	require.Equal(t, newExpectedDNSMsg(), result)

	result, err = lbClient.Query(context.Background(), []byte("abc"))
	require.NoError(t, err)
	require.Equal(t, newExpectedDNSMsg(), result)
}

func TestLoadBalanceDNSClientMaxFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		maxFails         int
		callCountClient1 int
		callCountClient2 int
	}{
		{
			name:             "OneMaxFail",
			maxFails:         1,
			callCountClient1: 1,
			callCountClient2: 0,
		},
		{
			name:             "TenMaxFails",
			maxFails:         10,
			callCountClient1: 1,
			callCountClient2: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			client1 := NewMockdnsClient(ctrl)
			client2 := NewMockdnsClient(ctrl)
			client1.EXPECT().Query(gomock.Any(), []byte("abc")).Return(newExpectedDNSMsg(), errors.New("client error")).Times(tt.callCountClient1)
			client2.EXPECT().Query(gomock.Any(), []byte("abc")).Return(newExpectedDNSMsg(), errors.New("client error")).Times(tt.callCountClient2)
			clients := []dnsClient{client1, client2}

			lbClient := newLoadBalanceDNSClient(clients,
				withLbPolicy(newSequentialPolicy()),
				withLbRequestTimeout(defaultRequestTimeout),
				withLbMaxFails(tt.maxFails))

			result, err := lbClient.Query(context.Background(), []byte("abc"))
			require.Error(t, err)
			require.Equal(t, newExpectedDNSMsg(), result)
		})
	}
}

func TestDefaultNewLoadBalanceDNSClient(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client1 := NewMockdnsClient(ctrl)
	client2 := NewMockdnsClient(ctrl)
	clients := []dnsClient{client1, client2}

	lbClient := newLoadBalanceDNSClient(clients)
	require.Equal(t, 2, lbClient.maxFails)
	require.Equal(t, defaultRequestTimeout, lbClient.timeout)
}
