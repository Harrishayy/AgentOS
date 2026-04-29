package manifest

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var nameRe = regexp.MustCompile(`^[a-z0-9-]{1,63}$`)

// validateName checks the DNS-label compatibility rule.
func validateName(eb *errBuilder, n *yaml.Node, v string) {
	if !nameRe.MatchString(v) {
		eb.addf(CodeInvalidName, n.Line, n.Column, "name",
			"name %q is invalid: must match [a-z0-9-]{1,63}", v)
	}
}

// validateHosts validates each entry in allowed_hosts against the v1 host
// pattern grammar (INTERFACES §1.3):
//
//   - hostname literal: api.openai.com
//   - wildcard left-most label: *.openai.com
//   - IP literal: 203.0.113.5 / 2001:db8::1
//   - CIDR: 10.0.0.0/8 / 2001:db8::/32
//   - any of the above with optional :port suffix.
func validateHosts(eb *errBuilder, n *yaml.Node, hosts []string) {
	for i, h := range hosts {
		if !validHostPattern(h) {
			line, col := childLineCol(n, i)
			eb.addf(CodeInvalidHostPattern, line, col,
				fmt.Sprintf("allowed_hosts[%d]", i),
				"%q is not a valid host pattern; expected hostname (api.example.com), IP, or wildcard (*.example.com), optionally with :port",
				h)
		}
	}
}

// validHostPattern returns true if h matches the v1 host pattern grammar.
func validHostPattern(h string) bool {
	if h == "" {
		return false
	}
	host, port, hasPort := splitHostPort(h)
	if hasPort {
		p, err := strconv.Atoi(port)
		if err != nil || p <= 0 || p > 65535 {
			return false
		}
	}
	return validHostBody(host)
}

// validHostBody checks the host portion (without :port). Order matters:
// CIDR check must precede IP check (IP literal can be a CIDR's prefix).
func validHostBody(s string) bool {
	if s == "" {
		return false
	}
	// CIDR (with /N).
	if strings.Contains(s, "/") {
		_, ipnet, err := net.ParseCIDR(s)
		if err != nil {
			return false
		}
		// ParseCIDR also rejects /33 (etc.) and host-bits-set automatically:
		// the canonical form must equal the input's masked address.
		// Verify host bits are zero by comparing the parsed network to the input IP.
		ipPart := strings.SplitN(s, "/", 2)[0]
		ip := net.ParseIP(ipPart)
		if ip == nil {
			return false
		}
		// Compare masked vs raw: host bits set ⇒ they differ.
		masked := ip.Mask(ipnet.Mask)
		if !masked.Equal(ip) {
			return false
		}
		return true
	}
	// Plain IP.
	if ip := net.ParseIP(s); ip != nil {
		return true
	}
	// Wildcard left-most label: *.<rest>
	if strings.HasPrefix(s, "*.") {
		return validHostnameLiteral(s[2:])
	}
	// Hostname literal.
	return validHostnameLiteral(s)
}

var hostnameLabelRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

// validHostnameLiteral checks RFC 1123-ish hostname syntax.
func validHostnameLiteral(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if !hostnameLabelRe.MatchString(label) {
			return false
		}
	}
	return true
}

// splitHostPort splits "<host>:<port>" while tolerating IPv6 literals (which
// must be wrapped in [] when a port is present).
func splitHostPort(s string) (host, port string, hasPort bool) {
	// Bracketed IPv6 with port: [::1]:443
	if strings.HasPrefix(s, "[") {
		closeIdx := strings.Index(s, "]")
		if closeIdx == -1 {
			return s, "", false
		}
		host = s[1:closeIdx]
		rest := s[closeIdx+1:]
		if strings.HasPrefix(rest, ":") {
			return host, rest[1:], true
		}
		return host, "", false
	}
	// IPv6 literal without brackets and without port (contains multiple ':').
	if strings.Count(s, ":") > 1 {
		return s, "", false
	}
	if i := strings.LastIndex(s, ":"); i != -1 {
		return s[:i], s[i+1:], true
	}
	return s, "", false
}

// validatePaths checks each entry in allowed_paths against the v1 grammar
// (INTERFACES §1.3): absolute path, optionally trailing-slash directory, or a
// path containing exactly one '*' glob (no '**', '?', or character classes).
func validatePaths(eb *errBuilder, n *yaml.Node, paths []string) {
	for i, p := range paths {
		if !validPathPattern(p) {
			line, col := childLineCol(n, i)
			eb.addf(CodeInvalidPathPattern, line, col,
				fmt.Sprintf("allowed_paths[%d]", i),
				"%q is not a valid path pattern; expected absolute path, '/dir/' for tree, or single '*' glob",
				p)
		}
	}
}

// validPathPattern enforces the v1 grammar:
//
//   - must be absolute (start with '/')
//   - no embedded NUL/LF/CR/TAB (BPF allowlist key behaviour with these is
//     kernel-implementation-defined; reject up front)
//   - no '**', '?', '[', ']'
//   - at most one '*' wildcard
func validPathPattern(p string) bool {
	if !strings.HasPrefix(p, "/") {
		return false
	}
	if strings.ContainsAny(p, "\x00\n\r\t") {
		return false
	}
	if strings.Contains(p, "**") {
		return false
	}
	if strings.ContainsAny(p, "?[]") {
		return false
	}
	if strings.Count(p, "*") > 1 {
		return false
	}
	return true
}

// validUser accepts a numeric uid (>= 0, <= MaxInt32) or a non-empty
// username/groupname-style string. We don't verify the uid exists at validation
// time — that's a daemon concern (the daemon resolves via NSS at clone3 time).
// We just check shape and reject negatives so users get a useful error before
// the kernel rejects setuid(-1).
func validUser(s string) bool {
	if s == "" {
		return false
	}
	if uid, err := strconv.Atoi(s); err == nil {
		return uid >= 0 && uid <= 2147483647
	}
	return regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]{0,31}$`).MatchString(s)
}

// validStdin accepts "inherit", "close", or "file:<abs-path>". The file:
// variant rejects embedded NUL/LF/CR/TAB so the path can't smuggle control
// characters into the daemon's open(2) call.
func validStdin(s string) bool {
	if s == "inherit" || s == "close" {
		return true
	}
	if strings.HasPrefix(s, "file:") && strings.HasPrefix(s[5:], "/") {
		if strings.ContainsAny(s[5:], "\x00\n\r\t") {
			return false
		}
		return true
	}
	return false
}

// childLineCol returns the line/col of the i'th child in a yaml.SequenceNode,
// falling back to the sequence's own line/col if the index is out of range.
func childLineCol(n *yaml.Node, i int) (int, int) {
	if n == nil {
		return 0, 0
	}
	if i >= 0 && i < len(n.Content) {
		c := n.Content[i]
		return c.Line, c.Column
	}
	return n.Line, n.Column
}
