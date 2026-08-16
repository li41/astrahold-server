# S4-F.17 — Trusted Reverse-Proxy Source Attribution / Edge Trust Boundary

## Scope

S4-F.17 adds an optional, explicit trust boundary for the source-IP throttles on the session-login / public-recovery HTTPS control plane.

The default deployment remains unchanged: login and recovery use the actual TLS socket peer from `Request.RemoteAddr`, and arbitrary forwarding headers are ignored.

When an operator intentionally places `worldd` behind a trusted reverse proxy, S4-F.17 can attribute login/recovery attempts to the original client address without trusting forwarding metadata from the public Internet.

The public login/recovery API, Client F.11 UX, durable account schema v4, F.12–F.15 recovery delivery/outbox contracts, F.16 TLS certificate generations, trusted game bootstrap, gameplay authority, and Protocol v9 remain unchanged.

## Deployment flags

Source attribution is disabled unless both flags are configured:

```text
-session-login-trusted-proxy-cidrs
-session-login-forwarded-header
```

`-session-login-trusted-proxy-cidrs` is a comma-separated allowlist of exact IPv4/IPv6 addresses or CIDR prefixes. At most 64 normalized prefixes are accepted.

`-session-login-forwarded-header` must be exactly one of:

```text
x-forwarded-for
forwarded
```

The configured header is the single authoritative forwarding format for that deployment. The other header is ignored rather than merged with it.

If only one flag is configured, an unsupported header mode is selected, or an allowlist entry is malformed, startup fails closed.

## Trust boundary

The TLS socket peer is always evaluated first.

```text
HTTP POST
→ parse actual socket RemoteAddr
→ is socket peer in operator trusted-proxy allowlist?
   ├── no  → ignore X-Forwarded-For / Forwarded completely
   │         throttle on actual socket peer
   └── yes → require one bounded configured forwarding field
             parse + normalize chain
             derive original client boundary
             throttle on attributed source
```

An untrusted/direct peer cannot gain another throttle bucket by sending, changing, or corrupting `X-Forwarded-For` or `Forwarded`.

A trusted proxy is expected to sanitize/replace its configured forwarding metadata according to the deployment topology. S4-F.17 does not authenticate arbitrary public header content independently of the trusted socket peer; the allowlist is the edge trust decision.

## Multi-hop selection

The header chain is interpreted from the nearest advertised hop back toward the original client.

The real socket peer is already known to be trusted. The Server walks the configured forwarding chain right-to-left:

1. trusted intermediary proxy hops are stripped;
2. the first untrusted address becomes the attributed source;
3. if every advertised hop is trusted, the left-most address is treated as the originating address for that bounded chain.

Example:

```text
socket peer                         127.0.0.2   trusted
X-Forwarded-For                     198.51.100.20, 10.0.0.9
trusted allowlist                   127.0.0.2/32, 10.0.0.0/8

right-most advertised 10.0.0.9     trusted → strip
next 198.51.100.20                  untrusted → attributed client
```

This prevents an untrusted intermediate proxy from supplying a spoofable address farther to the left: once the right-to-left walk reaches an untrusted boundary, earlier values are not consulted.

## Parsing and bounds

The selected forwarding field is accepted only from an allowlisted socket peer and is bounded to:

```text
maximum header bytes     1024
maximum hops             16
maximum trusted prefixes 64
```

A trusted peer must provide exactly one field instance of the configured header. Missing, empty, duplicated, oversized, malformed, or over-depth metadata is rejected before the login password KDF or recovery provider is invoked.

The public failure shape is the existing generic:

```text
HTTP 400
{"error":"invalid_request"}
```

This is intentionally fail-closed. The Server does not silently fall back to the proxy IP after an explicitly trusted proxy supplies invalid metadata.

### X-Forwarded-For mode

Each comma-separated hop must be a bare IPv4 or IPv6 literal. Hostnames, ports, empty elements, and non-IP tokens are rejected.

### Forwarded mode

Each comma-separated element must contain exactly one `for=` parameter. Other syntactically formed parameters may be present and are ignored for attribution.

The reference parser accepts IP literals and conventional `for=` address+port forms, including quoted bracketed IPv6. `for=unknown`, obfuscated identifiers such as `for=_hidden`, duplicate `for=` parameters, elements without `for=`, and malformed quoting fail closed.

## Address normalization

All addresses are normalized through Go `netip` before comparison or bucket selection.

IPv4-mapped IPv6 values are unmapped to canonical IPv4, so these cannot create separate throttle identities:

```text
::ffff:198.51.100.30
198.51.100.30
```

IPv4 and IPv6 trusted CIDRs are compared after the same normalization. Scoped IPv6 addresses/prefixes are not accepted for this control-plane attribution contract.

## Throttle semantics

S4-F.17 changes source selection only. The existing fixed-window guards remain unchanged:

- login and recovery keep independent attempt windows;
- the existing maximum tracked-source bound remains 4096 per guard;
- login throttling still occurs before password KDF work;
- recovery request/reset share the existing recovery guard;
- responses remain `429 login_throttled` or `429 recovery_throttled` with `Retry-After`.

Direct deployments therefore retain the exact pre-F.17 behavior.

## Observability

Startup logs report only bounded configuration metadata such as:

```text
source_attribution=socket
```

or:

```text
source_attribution=trusted-proxy/x-forwarded-for
trusted_proxy_prefixes=2
forwarded_max_hops=16
```

Normal logs do not emit attacker-controlled forwarding header values or attributed client IPs. F.17 therefore does not turn source attribution into a new request-address logging surface.

## Production acceptance

`Production Trusted Proxy Attribution E2E` uses the real `worldd` binary, a real TLS 1.3 control-plane listener, and a TLS reverse-proxy harness whose upstream socket is explicitly bound to an allowlisted loopback address.

It proves:

1. a direct/untrusted socket peer cannot evade login throttling by changing malformed or valid `X-Forwarded-For` values;
2. two clients behind the allowlisted proxy receive independent login buckets;
3. a trusted multi-hop proxy chain is stripped right-to-left;
4. IPv4-mapped IPv6 and canonical IPv4 map to the same throttle identity;
5. malformed or missing forwarding metadata from the trusted proxy returns generic `400 invalid_request` before authentication;
6. recovery throttling uses the same attributed source boundary while retaining its independent guard;
7. ordinary Server logs do not contain request secrets or attacker-controlled forwarding values;
8. TLS remains 1.3 and Protocol remains v9.

The exact F.17 product head must also pass Server CI plus the existing F.9/F.10/F.12/F.13/F.14/F.15/F.16 production gates.

## Non-goals

S4-F.17 deliberately does not add:

- trust in forwarding headers from non-allowlisted peers;
- PROXY protocol support;
- dynamic/SIGHUP trusted-proxy allowlist reload;
- reverse-proxy authentication or mTLS proxy identity;
- WAF/CDN/vendor-specific integration;
- distributed rate limiting;
- IP reputation, credential-stuffing intelligence, or CAPTCHA;
- Client-controlled source address or proxy metadata;
- Client CA/trust-policy hot reload;
- ACME/PKI automation;
- distributed account/session storage;
- public registration, MFA/WebAuthn/passkeys/OIDC;
- refresh tokens or remembered sessions;
- Protocol v10, DTLS, QUIC, or gameplay changes.

## Resulting boundary

After F.17, the direct listener remains secure-by-default while a deliberately configured reverse-proxy deployment can preserve per-client login/recovery abuse-control buckets. Forwarding metadata becomes authoritative only after the actual socket peer crosses an operator-owned allowlist boundary; it never becomes a public Client authority surface.
