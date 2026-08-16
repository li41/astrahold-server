package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
)

const (
	sessionSourceAttributionMaxHeaderBytes     = 1024
	sessionSourceAttributionMaxHops            = 16
	sessionSourceAttributionMaxTrustedPrefixes = 64
)

var (
	sessionLoginTrustedProxyCIDRs = flag.String(
		"session-login-trusted-proxy-cidrs",
		"",
		"Optional comma-separated trusted reverse-proxy IP/CIDR allowlist for login/recovery source attribution; requires -session-login-forwarded-header",
	)
	sessionLoginForwardedHeader = flag.String(
		"session-login-forwarded-header",
		"",
		"Forwarded source header trusted only from allowlisted proxy peers: x-forwarded-for or forwarded",
	)
)

var errSessionSourceAttribution = errors.New("worldd: invalid session source attribution")

type sessionSourceHeaderMode uint8

const (
	sessionSourceHeaderNone sessionSourceHeaderMode = iota
	sessionSourceHeaderXForwardedFor
	sessionSourceHeaderForwarded
)

type sessionSourceAttributor struct {
	mode            sessionSourceHeaderMode
	headerName      string
	trustedPrefixes []netip.Prefix
}

func loadSessionSourceAttributor() (*sessionSourceAttributor, error) {
	return newSessionSourceAttributor(*sessionLoginForwardedHeader, *sessionLoginTrustedProxyCIDRs)
}

func newSessionSourceAttributor(headerMode, trustedCIDRs string) (*sessionSourceAttributor, error) {
	headerMode = strings.ToLower(strings.TrimSpace(headerMode))
	trustedCIDRs = strings.TrimSpace(trustedCIDRs)
	if headerMode == "" && trustedCIDRs == "" {
		return nil, nil
	}
	if headerMode == "" || trustedCIDRs == "" {
		return nil, fmt.Errorf("%w: session-login-forwarded-header and session-login-trusted-proxy-cidrs must be set together", errSessionSourceAttribution)
	}

	var mode sessionSourceHeaderMode
	var headerName string
	switch headerMode {
	case "x-forwarded-for":
		mode = sessionSourceHeaderXForwardedFor
		headerName = "X-Forwarded-For"
	case "forwarded":
		mode = sessionSourceHeaderForwarded
		headerName = "Forwarded"
	default:
		return nil, fmt.Errorf("%w: session-login-forwarded-header must be x-forwarded-for or forwarded", errSessionSourceAttribution)
	}

	items := strings.Split(trustedCIDRs, ",")
	if len(items) == 0 || len(items) > sessionSourceAttributionMaxTrustedPrefixes {
		return nil, fmt.Errorf("%w: trusted proxy allowlist must contain 1..%d entries", errSessionSourceAttribution, sessionSourceAttributionMaxTrustedPrefixes)
	}
	trusted := make([]netip.Prefix, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		prefix, err := parseSessionTrustedProxyPrefix(item)
		if err != nil {
			return nil, err
		}
		key := prefix.String()
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		trusted = append(trusted, prefix)
	}
	if len(trusted) == 0 {
		return nil, fmt.Errorf("%w: trusted proxy allowlist is empty", errSessionSourceAttribution)
	}
	return &sessionSourceAttributor{mode: mode, headerName: headerName, trustedPrefixes: trusted}, nil
}

func parseSessionTrustedProxyPrefix(value string) (netip.Prefix, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return netip.Prefix{}, fmt.Errorf("%w: empty trusted proxy entry", errSessionSourceAttribution)
	}
	if address, err := netip.ParseAddr(value); err == nil {
		address = address.Unmap()
		if address.Zone() != "" {
			return netip.Prefix{}, fmt.Errorf("%w: scoped trusted proxy address is not supported", errSessionSourceAttribution)
		}
		bits := 128
		if address.Is4() {
			bits = 32
		}
		return netip.PrefixFrom(address, bits), nil
	}
	prefix, err := netip.ParsePrefix(value)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%w: invalid trusted proxy %q", errSessionSourceAttribution, value)
	}
	address := prefix.Addr()
	if address.Zone() != "" {
		return netip.Prefix{}, fmt.Errorf("%w: scoped trusted proxy prefix is not supported", errSessionSourceAttribution)
	}
	if address.Is4In6() {
		if prefix.Bits() < 96 {
			return netip.Prefix{}, fmt.Errorf("%w: IPv4-mapped prefix must be at least /96", errSessionSourceAttribution)
		}
		prefix = netip.PrefixFrom(address.Unmap(), prefix.Bits()-96)
	}
	return prefix.Masked(), nil
}

func (a *sessionSourceAttributor) trusted(address netip.Addr) bool {
	if a == nil || !address.IsValid() {
		return false
	}
	address = address.Unmap()
	for _, prefix := range a.trustedPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func (a *sessionSourceAttributor) sourceIP(request *http.Request) (string, error) {
	if request == nil {
		return "", fmt.Errorf("%w: nil request", errSessionSourceAttribution)
	}
	peerText := sessionLoginSourceIP(request.RemoteAddr)
	peer, err := netip.ParseAddr(peerText)
	if err != nil {
		return "", fmt.Errorf("%w: invalid socket peer", errSessionSourceAttribution)
	}
	peer = peer.Unmap()
	if !a.trusted(peer) {
		return peer.String(), nil
	}

	values := request.Header.Values(a.headerName)
	if len(values) != 1 {
		return "", fmt.Errorf("%w: trusted proxy must provide exactly one %s field", errSessionSourceAttribution, a.headerName)
	}
	value := strings.TrimSpace(values[0])
	if value == "" || len(value) > sessionSourceAttributionMaxHeaderBytes {
		return "", fmt.Errorf("%w: trusted proxy %s field is empty or too large", errSessionSourceAttribution, a.headerName)
	}

	var chain []netip.Addr
	switch a.mode {
	case sessionSourceHeaderXForwardedFor:
		chain, err = parseSessionXForwardedFor(value)
	case sessionSourceHeaderForwarded:
		chain, err = parseSessionForwarded(value)
	default:
		err = fmt.Errorf("%w: unsupported forwarding mode", errSessionSourceAttribution)
	}
	if err != nil {
		return "", err
	}
	if len(chain) == 0 {
		return "", fmt.Errorf("%w: forwarding chain is empty", errSessionSourceAttribution)
	}

	// The TCP/TLS socket peer is already trusted. Walk the header from the
	// nearest advertised hop toward the original client. Trusted intermediaries
	// are stripped; the first untrusted hop is the client boundary. If every
	// advertised hop is trusted, the left-most hop is still the originating
	// address for this bounded chain.
	for index := len(chain) - 1; index >= 0; index-- {
		candidate := chain[index].Unmap()
		if !a.trusted(candidate) {
			return candidate.String(), nil
		}
	}
	return chain[0].Unmap().String(), nil
}

func (a *sessionSourceAttributor) wrap(next http.Handler) http.Handler {
	if a == nil || next == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request == nil || request.Method != http.MethodPost {
			next.ServeHTTP(w, request)
			return
		}
		source, err := a.sourceIP(request)
		if err != nil {
			writeSessionLoginError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		attributed := request.Clone(request.Context())
		attributed.RemoteAddr = source
		next.ServeHTTP(w, attributed)
	})
}

func sessionSourceAttributionMetadata(a *sessionSourceAttributor) (string, int) {
	if a == nil {
		return "socket", 0
	}
	switch a.mode {
	case sessionSourceHeaderXForwardedFor:
		return "trusted-proxy/x-forwarded-for", len(a.trustedPrefixes)
	case sessionSourceHeaderForwarded:
		return "trusted-proxy/forwarded", len(a.trustedPrefixes)
	default:
		return "invalid", len(a.trustedPrefixes)
	}
}

func parseSessionXForwardedFor(value string) ([]netip.Addr, error) {
	parts := strings.Split(value, ",")
	if len(parts) == 0 || len(parts) > sessionSourceAttributionMaxHops {
		return nil, fmt.Errorf("%w: X-Forwarded-For must contain 1..%d hops", errSessionSourceAttribution, sessionSourceAttributionMaxHops)
	}
	chain := make([]netip.Addr, 0, len(parts))
	for _, part := range parts {
		address, err := parseSessionForwardedAddress(strings.TrimSpace(part), false)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid X-Forwarded-For hop", errSessionSourceAttribution)
		}
		chain = append(chain, address)
	}
	return chain, nil
}

func parseSessionForwarded(value string) ([]netip.Addr, error) {
	elements, err := splitSessionForwarded(value, ',')
	if err != nil || len(elements) == 0 || len(elements) > sessionSourceAttributionMaxHops {
		return nil, fmt.Errorf("%w: Forwarded must contain 1..%d valid elements", errSessionSourceAttribution, sessionSourceAttributionMaxHops)
	}
	chain := make([]netip.Addr, 0, len(elements))
	for _, element := range elements {
		parameters, err := splitSessionForwarded(element, ';')
		if err != nil || len(parameters) == 0 {
			return nil, fmt.Errorf("%w: malformed Forwarded element", errSessionSourceAttribution)
		}
		var found bool
		var source netip.Addr
		for _, parameter := range parameters {
			pair := strings.SplitN(strings.TrimSpace(parameter), "=", 2)
			if len(pair) != 2 || strings.TrimSpace(pair[0]) == "" || strings.TrimSpace(pair[1]) == "" {
				return nil, fmt.Errorf("%w: malformed Forwarded parameter", errSessionSourceAttribution)
			}
			if !strings.EqualFold(strings.TrimSpace(pair[0]), "for") {
				continue
			}
			if found {
				return nil, fmt.Errorf("%w: duplicate Forwarded for parameter", errSessionSourceAttribution)
			}
			found = true
			value := strings.TrimSpace(pair[1])
			if strings.HasPrefix(value, "\"") {
				decoded, err := strconv.Unquote(value)
				if err != nil {
					return nil, fmt.Errorf("%w: invalid quoted Forwarded for value", errSessionSourceAttribution)
				}
				value = decoded
			} else if strings.Contains(value, "\"") {
				return nil, fmt.Errorf("%w: malformed Forwarded for value", errSessionSourceAttribution)
			}
			if strings.EqualFold(value, "unknown") || strings.HasPrefix(value, "_") {
				return nil, fmt.Errorf("%w: obfuscated Forwarded for value is not supported", errSessionSourceAttribution)
			}
			source, err = parseSessionForwardedAddress(value, true)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid Forwarded for address", errSessionSourceAttribution)
			}
		}
		if !found {
			return nil, fmt.Errorf("%w: Forwarded element has no for parameter", errSessionSourceAttribution)
		}
		chain = append(chain, source)
	}
	return chain, nil
}

func splitSessionForwarded(value string, separator byte) ([]string, error) {
	var parts []string
	start := 0
	quoted := false
	escaped := false
	for index := 0; index < len(value); index++ {
		current := value[index]
		if current == '\r' || current == '\n' || current == 0 {
			return nil, fmt.Errorf("%w: control character in Forwarded header", errSessionSourceAttribution)
		}
		if escaped {
			escaped = false
			continue
		}
		if quoted && current == '\\' {
			escaped = true
			continue
		}
		if current == '"' {
			quoted = !quoted
			continue
		}
		if !quoted && current == separator {
			part := strings.TrimSpace(value[start:index])
			if part == "" {
				return nil, fmt.Errorf("%w: empty Forwarded component", errSessionSourceAttribution)
			}
			parts = append(parts, part)
			start = index + 1
		}
	}
	if quoted || escaped {
		return nil, fmt.Errorf("%w: unterminated Forwarded quoted string", errSessionSourceAttribution)
	}
	part := strings.TrimSpace(value[start:])
	if part == "" {
		return nil, fmt.Errorf("%w: empty Forwarded component", errSessionSourceAttribution)
	}
	parts = append(parts, part)
	return parts, nil
}

func parseSessionForwardedAddress(value string, allowPort bool) (netip.Addr, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return netip.Addr{}, errSessionSourceAttribution
	}
	if address, err := netip.ParseAddr(value); err == nil {
		address = address.Unmap()
		if address.Zone() != "" {
			return netip.Addr{}, errSessionSourceAttribution
		}
		return address, nil
	}
	if !allowPort {
		return netip.Addr{}, errSessionSourceAttribution
	}
	if addressPort, err := netip.ParseAddrPort(value); err == nil {
		address := addressPort.Addr().Unmap()
		if address.Zone() != "" {
			return netip.Addr{}, errSessionSourceAttribution
		}
		return address, nil
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		address, err := netip.ParseAddr(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
		if err == nil {
			address = address.Unmap()
			if address.Zone() == "" {
				return address, nil
			}
		}
	}
	return netip.Addr{}, errSessionSourceAttribution
}
