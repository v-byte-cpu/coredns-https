package https

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

const upstreamURL = "https://example.com/dns-query"

type mockHTTPClientFunc func(*http.Request) (*http.Response, error)

func (f mockHTTPClientFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

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
	callCount := 0
	expectedMsg := newExpectedDNSMsg()
	httpClient := mockHTTPClientFunc(func(req *http.Request) (resp *http.Response, err error) {
		callCount++
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
			Body:       ioutil.NopCloser(bytes.NewReader(packMsg(t, expectedMsg))),
			StatusCode: http.StatusOK,
		}
		return
	})
	dnsClient := newDoHDNSClient(httpClient, upstreamURL)

	result, err := dnsClient.Query(context.Background(), []byte("abc"))
	require.NoError(t, err)
	require.Equal(t, 1, callCount, "invalid http client call count")
	require.True(t, result.MsgHdr.Response)
	require.Equal(t, len(expectedMsg.Answer), len(result.Answer))
	require.Equal(t, expectedMsg.Answer[0].String(), result.Answer[0].String())
}

func TestDNSClientNewRequestError(t *testing.T) {
	invalidURL := "https://example.com/\t\n"
	httpClient := mockHTTPClientFunc(func(req *http.Request) (resp *http.Response, err error) {
		t.Fatal("http client must not be called")
		return
	})
	dnsClient := newDoHDNSClient(httpClient, invalidURL)

	_, err := dnsClient.Query(context.Background(), []byte("abc"))
	require.Error(t, err)
}

func TestDNSClientHttpClientError(t *testing.T) {
	httpClient := mockHTTPClientFunc(func(req *http.Request) (resp *http.Response, err error) {
		err = errors.New("http error")
		resp = &http.Response{
			Body:       ioutil.NopCloser(bytes.NewReader(packMsg(t, newExpectedDNSMsg()))),
			StatusCode: http.StatusOK,
		}
		return
	})
	dnsClient := newDoHDNSClient(httpClient, upstreamURL)

	_, err := dnsClient.Query(context.Background(), []byte("abc"))
	require.Error(t, err)
}

func TestDNSClientHttpStatusError(t *testing.T) {
	httpClient := mockHTTPClientFunc(func(req *http.Request) (resp *http.Response, err error) {
		resp = &http.Response{
			Body:       ioutil.NopCloser(bytes.NewReader(packMsg(t, newExpectedDNSMsg()))),
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
	httpClient := mockHTTPClientFunc(func(req *http.Request) (resp *http.Response, err error) {
		reader := &errReader{
			delegate: bytes.NewReader(packMsg(t, newExpectedDNSMsg())),
			err:      errors.New("io error"),
		}
		resp = &http.Response{
			Body:       ioutil.NopCloser(reader),
			StatusCode: http.StatusOK,
		}
		return
	})
	dnsClient := newDoHDNSClient(httpClient, upstreamURL)

	_, err := dnsClient.Query(context.Background(), []byte("abc"))
	require.Error(t, err)
}

func TestDNSClientMsgUnpackError(t *testing.T) {
	httpClient := mockHTTPClientFunc(func(req *http.Request) (resp *http.Response, err error) {
		resp = &http.Response{
			Body:       ioutil.NopCloser(strings.NewReader("def")),
			StatusCode: http.StatusOK,
		}
		return
	})
	dnsClient := newDoHDNSClient(httpClient, upstreamURL)

	_, err := dnsClient.Query(context.Background(), []byte("abc"))
	require.Error(t, err)
}

func TestDNSClientLargeResponseError(t *testing.T) {
	//create large buffer with the correct DNS message at the beginning
	var buf bytes.Buffer
	dnsMsg := packMsg(t, newExpectedDNSMsg())
	buf.Write(dnsMsg)
	buf.WriteString(strings.Repeat("a", maxDNSMessageSize))

	httpClient := mockHTTPClientFunc(func(req *http.Request) (resp *http.Response, err error) {
		resp = &http.Response{
			Body:       ioutil.NopCloser(&buf),
			StatusCode: http.StatusOK,
		}
		return
	})
	dnsClient := newDoHDNSClient(httpClient, upstreamURL)

	_, err := dnsClient.Query(context.Background(), []byte("abc"))
	require.Error(t, err)
}

type mockDNSClient struct {
	callCount int
	reqBody   []byte
	t         *testing.T
	err       error
}

func (c *mockDNSClient) Query(ctx context.Context, dnsreq []byte) (result *dns.Msg, err error) {
	c.t.Helper()
	c.callCount++
	require.NotNil(c.t, ctx)
	require.Equal(c.t, c.reqBody, dnsreq, "invalid request body")
	return newExpectedDNSMsg(), c.err
}

func TestLoadBalanceDNSClient(t *testing.T) {
	client1 := &mockDNSClient{reqBody: []byte("abc"), t: t}
	clients := []dnsClient{client1}
	lbClient := newLoadBalanceDNSClient(clients)

	result, err := lbClient.Query(context.Background(), []byte("abc"))
	require.NoError(t, err)
	require.Equal(t, 1, client1.callCount)
	require.Equal(t, newExpectedDNSMsg(), result)
}

func TestLoadBalanceDNSClientError(t *testing.T) {
	client1 := &mockDNSClient{reqBody: []byte("abc"), t: t, err: errors.New("client error")}
	clients := []dnsClient{client1}
	lbClient := newLoadBalanceDNSClient(clients)

	_, err := lbClient.Query(context.Background(), []byte("abc"))
	require.Error(t, err)
	require.Equal(t, 1, client1.callCount)
}

func TestLoadBalanceDNSClientRetry(t *testing.T) {
	client1 := &mockDNSClient{reqBody: []byte("abc"), t: t, err: errors.New("client error")}
	client2 := &mockDNSClient{reqBody: []byte("abc"), t: t}
	clients := []dnsClient{client1, client2}
	lbClient := newLoadBalanceDNSClient(clients, withLbPolicy(newSequentialPolicy()))

	result, err := lbClient.Query(context.Background(), []byte("abc"))
	require.NoError(t, err)
	require.Equal(t, 1, client1.callCount)
	require.Equal(t, 1, client2.callCount)
	require.Equal(t, newExpectedDNSMsg(), result)
}

func TestLoadBalanceDNSClientPolicy(t *testing.T) {
	client1 := &mockDNSClient{reqBody: []byte("abc"), t: t}
	client2 := &mockDNSClient{reqBody: []byte("abc"), t: t}
	clients := []dnsClient{client1, client2}
	lbClient := newLoadBalanceDNSClient(clients, withLbPolicy(newRoundRobinPolicy()))

	result, err := lbClient.Query(context.Background(), []byte("abc"))
	require.NoError(t, err)
	require.Equal(t, 1, client1.callCount)
	require.Equal(t, 0, client2.callCount)
	require.Equal(t, newExpectedDNSMsg(), result)

	result, err = lbClient.Query(context.Background(), []byte("abc"))
	require.NoError(t, err)
	require.Equal(t, 1, client1.callCount)
	require.Equal(t, 1, client2.callCount)
	require.Equal(t, newExpectedDNSMsg(), result)
}

func TestLoadBalanceDNSClientMaxFails(t *testing.T) {
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
			client1 := &mockDNSClient{reqBody: []byte("abc"), t: t, err: errors.New("client error")}
			client2 := &mockDNSClient{reqBody: []byte("abc"), t: t, err: errors.New("client error")}
			clients := []dnsClient{client1, client2}

			lbClient := newLoadBalanceDNSClient(clients,
				withLbPolicy(newSequentialPolicy()),
				withLbMaxFails(tt.maxFails))

			result, err := lbClient.Query(context.Background(), []byte("abc"))
			require.Error(t, err)
			require.Equal(t, tt.callCountClient1, client1.callCount)
			require.Equal(t, tt.callCountClient2, client2.callCount)
			require.Equal(t, newExpectedDNSMsg(), result)
		})
	}
}

func TestDefaultNewLoadBalanceDNSClient(t *testing.T) {
	client1 := &mockDNSClient{reqBody: []byte("abc"), t: t}
	client2 := &mockDNSClient{reqBody: []byte("abc"), t: t}
	clients := []dnsClient{client1, client2}

	lbClient := newLoadBalanceDNSClient(clients)
	require.Equal(t, 2, lbClient.maxFails)
	require.Equal(t, defaultRequestTimeout, lbClient.timeout)
}
