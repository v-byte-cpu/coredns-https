# https

**https** is a [CoreDNS](https://github.com/coredns/coredns) plugin that proxies DNS messages to upstream resolvers using DNS-over-HTTPS protocol. See [RFC 8484](https://tools.ietf.org/html/rfc8484).

## Installation

External CoreDNS plugins can be enabled in one of two ways:
  1. [Build with compile-time configuration file](https://coredns.io/2017/07/25/compile-time-enabling-or-disabling-plugins/#build-with-compile-time-configuration-file)
  2. [Build with external golang source code](https://coredns.io/2017/07/25/compile-time-enabling-or-disabling-plugins/#build-with-external-golang-source-code)

Method #1 can be quickly described using a sequence of the following commands:

```
git clone --depth 1 https://github.com/coredns/coredns.git
cd coredns
go get github.com/v-byte-cpu/coredns-https
echo "https:github.com/v-byte-cpu/coredns-https" >> plugin.cfg
go generate
go mod tidy -compat=1.17
go build
```

## Syntax

In its most basic form:

~~~
https FROM TO...
~~~

* **FROM** is the base domain to match for the request to be proxied.
* **TO...** are the destination endpoints to proxy to. The number of upstreams is
  limited to 15.

Multiple upstreams are randomized (see `policy`) on first use. When a proxy returns an error
the next upstream in the list is tried.

Extra knobs are available with an expanded syntax:

~~~
https FROM TO... {
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

If monitoring is enabled (via the *prometheus* plugin) then the following metric are exported:

* `coredns_https_request_duration_seconds{to}` - duration per upstream interaction.
* `coredns_https_requests_total{to}` - query count per upstream.
* `coredns_https_responses_total{to, rcode}` - count of RCODEs per upstream.
  and we are randomly (this always uses the `random` policy) spraying to an upstream.

## Examples

Proxy all requests within `example.org.` to a DoH nameserver:

~~~ corefile
example.org {
    https . cloudflare-dns.com/dns-query
}
~~~

Forward everything except requests to `example.org`

~~~ corefile
. {
    https . dns.quad9.net/dns-query {
        except example.org
    }
}
~~~

Load balance all requests between multiple upstreams

~~~ corefile
. {
    https . dns.quad9.net/dns-query cloudflare-dns.com:443/dns-query dns.google/dns-query
}
~~~

Internal DoH server:

~~~ corefile
. {
    https . 10.0.0.10:853/dns-query {
      tls ca.crt
      tls_servername internal.domain
    }
}
~~~