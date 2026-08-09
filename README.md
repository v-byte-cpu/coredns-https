# coredns-https

[![CI](https://github.com/v-byte-cpu/coredns-https/actions/workflows/ci.yml/badge.svg)](https://github.com/v-byte-cpu/coredns-https/actions/workflows/ci.yml)
[![GitHub release](https://img.shields.io/github/v/release/v-byte-cpu/coredns-https)](https://github.com/v-byte-cpu/coredns-https/releases/latest)

**coredns-https** provides the `dohproxy` [CoreDNS](https://github.com/coredns/coredns)
plugin, which proxies DNS messages to upstream resolvers using DNS-over-HTTPS.
See [RFC 8484](https://www.rfc-editor.org/rfc/rfc8484.html).

## Installation

The plugin must be compiled into CoreDNS. The following commands build the latest plugin release
against the tested CoreDNS version:

```sh
git clone --depth 1 --branch v1.14.6 https://github.com/coredns/coredns.git
cd coredns
go get github.com/v-byte-cpu/coredns-https@latest
COREDNS_PLUGINS="dohproxy:github.com/v-byte-cpu/coredns-https" go generate coredns.go
go build
./coredns -plugins | grep -Fx dohproxy
```

> [!IMPORTANT]
> The Corefile directive was renamed from `https` to `dohproxy` to avoid a collision with the
> built-in CoreDNS `https` plugin. Prometheus metrics were also renamed from
> `coredns_https_*` to `coredns_dohproxy_*`.

## Local development

Run the unit tests first:

```sh
go test ./...
```

To test the current checkout in a real CoreDNS binary, clone the tested CoreDNS version into a
separate directory and replace the module with the absolute path to this repository:

```sh
git clone --depth 1 --branch v1.14.6 https://github.com/coredns/coredns.git
cd coredns
go mod edit -replace github.com/v-byte-cpu/coredns-https=/absolute/path/to/coredns-https
go get github.com/v-byte-cpu/coredns-https@v0.0.0
COREDNS_PLUGINS="dohproxy:github.com/v-byte-cpu/coredns-https" go generate coredns.go
go build
./coredns -plugins | grep -Fx dohproxy
```

Create a `Corefile` for a manual end-to-end check:

~~~ corefile
.:1053 {
    dohproxy . cloudflare-dns.com/dns-query
    errors
    log
}
~~~

Start CoreDNS, then query it from another terminal:

```sh
./coredns -conf Corefile
dig @127.0.0.1 -p 1053 example.org A
```

## Syntax

In its most basic form:

~~~
dohproxy FROM TO...
~~~

* **FROM** is the base domain to match for the request to be proxied.
* **TO...** are the destination endpoints to proxy to. The number of upstreams is
  limited to 15.

Multiple upstreams are randomized (see `policy`) on first use. When a proxy returns an error
the next upstream in the list is tried.

Extra knobs are available with an expanded syntax:

~~~
dohproxy FROM TO... {
    except IGNORED_NAMES...
    tls CERT KEY CA
    tls_servername NAME
    policy random|round_robin|sequential
}
~~~

* **FROM** and **TO...** as above.
* **IGNORED_NAMES** in `except` is a space-separated list of domains to exclude from proxying.
  Requests that match none of these names will be passed through.
* `tls` **CERT** **KEY** **CA** define the TLS properties for TLS connection. From 0 to 3 arguments can be
  provided with the meaning as described below

  * `tls` - no client authentication is used, and the system CAs are used to verify the server certificate (by default)
  * `tls` **CA** - no client authentication is used, and the file CA is used to verify the server certificate
  * `tls` **CERT** **KEY** - client authentication is used with the specified cert/key pair.
    The server certificate is verified with the system CAs
  * `tls` **CERT** **KEY**  **CA** - client authentication is used with the specified cert/key pair.
    The server certificate is verified using the specified CA file

* `policy` specifies the policy to use for selecting upstream servers. The default is `random`.


## Metrics

If monitoring is enabled via the *prometheus* plugin, the following metrics are exported:

* `coredns_dohproxy_request_duration_seconds{to}` - duration per upstream interaction.
* `coredns_dohproxy_requests_total{to}` - query count per upstream.
* `coredns_dohproxy_responses_total{to, rcode}` - count of RCODEs per upstream.

## Examples

Proxy all requests within `example.org.` to a DoH nameserver:

~~~ corefile
example.org {
    dohproxy . cloudflare-dns.com/dns-query
}
~~~

Forward everything except requests to `example.org`

~~~ corefile
. {
    dohproxy . dns.quad9.net/dns-query {
        except example.org
    }
}
~~~

Load balance all requests between multiple upstreams

~~~ corefile
. {
    dohproxy . dns.quad9.net/dns-query cloudflare-dns.com:443/dns-query dns.google/dns-query
}
~~~

Internal DoH server:

~~~ corefile
. {
    dohproxy . 10.0.0.10:853/dns-query {
      tls ca.crt
      tls_servername internal.domain
    }
}
~~~
