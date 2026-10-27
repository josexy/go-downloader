package proxy

import (
	"strconv"
	"strings"
)

// parseWindowsProxy turns WinINet's ProxyServer value into endpoints. The value
// is either "host:port" for every protocol or "http=host:port;https=host:port;
// socks=host:port".
func parseWindowsProxy(enabled bool, server, override string) *System {
	sys := &System{Bypass: strings.TrimSpace(override)}
	if !enabled || strings.TrimSpace(server) == "" {
		return sys
	}
	if strings.Contains(server, "=") {
		for _, part := range strings.Split(server, ";") {
			key, value, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			ep, err := parseEndpoint(strings.TrimSpace(value), KindHTTP)
			if err != nil {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "http":
				sys.HTTP = ep
			case "https":
				sys.HTTPS = ep
			case "socks", "socks5", "socks4":
				socks := *ep
				socks.Kind = KindSOCKS5
				sys.SOCKS = &socks
			}
		}
		return sys
	}
	ep, err := parseEndpoint(strings.TrimSpace(server), KindHTTP)
	if err != nil {
		return sys
	}
	sys.HTTP, sys.HTTPS = ep, ep
	return sys
}

// parseScutil turns the output of `scutil --proxy` into endpoints.
func parseScutil(out string) *System {
	sys := &System{}
	values := map[string]string{}
	var exceptions []string
	inExceptions := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "ExceptionsList") {
			inExceptions = true
			continue
		}
		if inExceptions {
			if strings.HasPrefix(line, "}") {
				inExceptions = false
				continue
			}
			if _, value, ok := strings.Cut(line, ":"); ok {
				if v := strings.TrimSpace(value); v != "" {
					exceptions = append(exceptions, v)
				}
			}
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	makeEndpoint := func(prefix string, kind Kind) *Endpoint {
		if values[prefix+"Enable"] != "1" {
			return nil
		}
		host := values[prefix+"Proxy"]
		if host == "" {
			return nil
		}
		port := 0
		if p := values[prefix+"Port"]; p != "" {
			port, _ = strconv.Atoi(p)
		}
		if port < 1 || port > 65535 {
			switch kind {
			case KindHTTPS:
				port = 443
			case KindSOCKS5:
				port = 1080
			default:
				port = 80
			}
		}
		return &Endpoint{Kind: kind, Host: host, Port: port}
	}
	sys.HTTP = makeEndpoint("HTTP", KindHTTP)
	sys.HTTPS = makeEndpoint("HTTPS", KindHTTPS)
	sys.SOCKS = makeEndpoint("SOCKS", KindSOCKS5)
	sys.Bypass = strings.Join(exceptions, ",")
	return sys
}
