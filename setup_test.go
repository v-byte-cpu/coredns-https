package https

import (
	"crypto/tls"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/stretchr/testify/require"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedConfig *httpsConfig
	}{
		{
			name:  "FromAndOneToURL",
			input: "https . example.com/dns-query",
			expectedConfig: &httpsConfig{
				from:   ".",
				toURLs: []string{"https://example.com/dns-query"},
			},
		},
		{
			name:  "FromAndTwoToURLs",
			input: "https . example.com/dns-query example.org/dns-query",
			expectedConfig: &httpsConfig{
				from:   ".",
				toURLs: []string{"https://example.com/dns-query", "https://example.org/dns-query"},
			},
		},
		{
			name:  "ExceptProperty",
			input: "https . example.com/dns-query {\nexcept domain.com\n}\n",
			expectedConfig: &httpsConfig{
				from:   ".",
				toURLs: []string{"https://example.com/dns-query"},
				except: []string{"domain.com."},
			},
		},
		{
			name:  "ExceptPropertyURL",
			input: "https . example.com/dns-query {\nexcept https://domain.com\n}\n",
			expectedConfig: &httpsConfig{
				from:   ".",
				toURLs: []string{"https://example.com/dns-query"},
				except: []string{"domain.com."},
			},
		},
		{
			name:  "ExceptPropertyTwoDomains",
			input: "https . example.com/dns-query {\nexcept domain1.com domain2.com\n}\n",
			expectedConfig: &httpsConfig{
				from:   ".",
				toURLs: []string{"https://example.com/dns-query"},
				except: []string{"domain1.com.", "domain2.com."},
			},
		},
		{
			name:  "TLSServerNameProperty",
			input: "https . 10.1.1.1:853/dns-query {\ntls_servername internal.domain\n}\n",
			expectedConfig: &httpsConfig{
				from:          ".",
				toURLs:        []string{"https://10.1.1.1:853/dns-query"},
				tlsConfig:     &tls.Config{ServerName: "internal.domain"},
				tlsServerName: "internal.domain",
			},
		},
		{
			name:  "PolicyPropertyRandom",
			input: "https . example.com/dns-query {\npolicy random\n}\n",
			expectedConfig: &httpsConfig{
				from:   ".",
				toURLs: []string{"https://example.com/dns-query"},
				policy: newRandomPolicy(),
			},
		},
		{
			name:  "PolicyPropertyRoundRobin",
			input: "https . example.com/dns-query {\npolicy round_robin\n}\n",
			expectedConfig: &httpsConfig{
				from:   ".",
				toURLs: []string{"https://example.com/dns-query"},
				policy: newRoundRobinPolicy(),
			},
		},
		{
			name:  "PolicyPropertySequential",
			input: "https . example.com/dns-query {\npolicy sequential\n}\n",
			expectedConfig: &httpsConfig{
				from:   ".",
				toURLs: []string{"https://example.com/dns-query"},
				policy: newSequentialPolicy(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := caddy.NewTestController("https", tt.input)
			conf, err := parseConfig(c)
			require.NoError(t, err)
			require.Equal(t, tt.expectedConfig, conf)
		})
	}
}

func TestParseConfigTLSProperty(t *testing.T) {
	input := "https . example.com/dns-query {\ntls\n}\n"
	c := caddy.NewTestController("https", input)
	conf, err := parseConfig(c)
	require.NoError(t, err)
	require.Equal(t, ".", conf.from)
	require.Equal(t, []string{"https://example.com/dns-query"}, conf.toURLs)
	require.NotNil(t, conf.tlsConfig)
}

func TestParseConfigError(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "EmptyLine",
			input: "",
		},
		{
			name:  "EmptyFrom",
			input: "https",
		},
		{
			name:  "EmptyToURLs",
			input: "https .",
		},
		{
			name:  "InvalidToURL",
			input: "https . abc:&",
		},
		{
			name:  "TooManyToURLs",
			input: "https . " + strings.Repeat("example.com/dns-query ", maxUpstreams+1),
		},
		{
			name:  "UnknownProperty",
			input: "https . example.com/dns-query {\nabc\n}\n",
		},
		{
			name:  "ExceptPropertyZeroArgs",
			input: "https . example.com/dns-query {\nexcept\n}\n",
		},
		{
			name:  "TLSPropertyTooManyArgs",
			input: "https . example.com/dns-query {\ntls abc def ghi qwe\n}\n",
		},
		{
			name:  "TLSServerNamePropertyZeroArgs",
			input: "https . example.com/dns-query {\ntls_servername\n}\n",
		},
		{
			name:  "TLSServerNamePropertyTooManyArgs",
			input: "https . example.com/dns-query {\ntls_servername abc.com def.com\n}\n",
		},
		{
			name:  "PolicyPropertyZeroArgs",
			input: "https . example.com/dns-query {\npolicy\n}\n",
		},
		{
			name:  "PolicyPropertyTooManyArgs",
			input: "https . example.com/dns-query {\npolicy random round_robin\n}\n",
		},
		{
			name:  "PolicyPropertyUnknownArg",
			input: "https . example.com/dns-query {\npolicy abc\n}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := caddy.NewTestController("https", tt.input)
			_, err := parseConfig(c)
			require.Error(t, err)
		})
	}
}
