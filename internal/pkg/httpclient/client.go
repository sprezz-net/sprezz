package httpclient

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

const UserAgent = "Sprezz-Hex-QuadStore/2.0"

var privateIPBlocks []*net.IPNet

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8",        // IPv4 loopback
		"10.0.0.0/8",         // RFC1918
		"100.64.0.0/10",      // RFC6598 Carrier-Grade NAT
		"172.16.0.0/12",      // RFC1918
		"169.254.0.0/16",     // RFC3927 Link-Local
		"192.0.0.0/24",       // RFC6890
		"192.0.2.0/24",       // RFC5737 Test-Net-1
		"192.88.99.0/24",     // RFC3068 6to4 Relay Anycast
		"192.168.0.0/16",     // RFC1918
		"198.18.0.0/15",      // RFC2544 Benchmarking
		"198.51.100.0/24",    // RFC5737 Test-Net-2
		"203.0.113.0/24",     // RFC5737 Test-Net-3
		"224.0.0.0/4",        // RFC1112 Multicast
		"240.0.0.0/4",        // RFC1112 Reserved
		"255.255.255.255/32", // RFC919 Broadcast
		"::1/128",            // IPv6 loopback
		"fc00::/7",           // IPv6 Unique Local Address
		"fe80::/10",          // IPv6 Link-Local
		"ff00::/8",           // IPv6 Multicast
	} {
		_, block, err := net.ParseCIDR(cidr)
		if err == nil {
			privateIPBlocks = append(privateIPBlocks, block)
		}
	}
}

// userAgentTransport wraps an existing http.RoundTripper and injects the predefined User-Agent.
type userAgentTransport struct {
	base http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", UserAgent)
	return t.base.RoundTrip(req)
}

// isPrivateOrLocalIP returns true if the given IP address is a private, loopback,
// link-local, unspecified, or multicast address.
func isPrivateOrLocalIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	// Check standard library properties
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() || ip.IsMulticast() {
		return true
	}
	// Check comprehensive custom private network lists
	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

// safeDialContext wraps a dialer and adds strict validation of resolved IP addresses
// to prevent SSRF and DNS rebinding attacks.
func safeDialContext(dialer *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		// Resolve IP addresses for the host
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}

		if len(ips) == 0 {
			return nil, fmt.Errorf("no IP addresses found for host: %s", host)
		}

		// Ensure all resolved IPs are public / safe
		for _, ip := range ips {
			if isPrivateOrLocalIP(ip) {
				return nil, fmt.Errorf("connection to private or local address is forbidden: %s", ip.String())
			}
		}

		// Pin connection to the first resolved & validated IP address to prevent DNS rebinding
		targetIP := ips[0]
		targetAddr := net.JoinHostPort(targetIP.String(), port)

		return dialer.DialContext(ctx, network, targetAddr)
	}
}

// New returns an *http.Client configured with a 10-second timeout, the User-Agent transport,
// and a secure transport that protects against SSRF and DNS rebinding.
func New() *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 10 * time.Second,
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           safeDialContext(dialer),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &userAgentTransport{
			base: transport,
		},
	}
}
