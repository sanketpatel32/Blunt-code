package dast

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// PentestAllowPrivateEnv is the operator-side switch that lets the probe reach
// private-network and link-local targets. It is read from the environment of
// the process running the probe, never from the request body, so a caller of
// the loopback API cannot unlock internal ranges on its own.
const PentestAllowPrivateEnv = "BLUNTCODE_PENTEST_ALLOW_PRIVATE"

var (
	// ErrProbeTargetBlocked marks a probe refused because the target resolves
	// to an address outside the probe's network policy.
	ErrProbeTargetBlocked = errors.New("probe target blocked")
	// ErrCleartextCredentials marks a probe refused because caller-supplied
	// credentials would be sent over plain http://.
	ErrCleartextCredentials = errors.New("credentials refused over cleartext http")
)

// PentestAllowPrivateFromEnv reports whether PentestAllowPrivateEnv is set to
// a true value in getenv.
func PentestAllowPrivateFromEnv(getenv func(string) string) bool {
	if getenv == nil {
		getenv = os.Getenv
	}
	allow, err := strconv.ParseBool(strings.TrimSpace(getenv(PentestAllowPrivateEnv)))
	return err == nil && allow
}

type probeAddrClass int

const (
	probeAddrPublic probeAddrClass = iota
	probeAddrLoopback
	probeAddrPrivate
	probeAddrForbidden
)

// Cloud instance-metadata and credential endpoints. These hand out instance
// credentials to anything that can reach them, so they are refused even when
// private targets are allowed.
var probeMetadataAddrs = map[netip.Addr]string{
	netip.MustParseAddr("169.254.169.254"): "cloud instance metadata service",
	netip.MustParseAddr("169.254.170.2"):   "ECS task metadata/credentials endpoint",
	netip.MustParseAddr("169.254.170.23"):  "EKS pod identity credentials endpoint",
	netip.MustParseAddr("100.100.100.200"): "Alibaba Cloud metadata service",
	netip.MustParseAddr("fd00:ec2::254"):   "AWS IPv6 instance metadata service",
	netip.MustParseAddr("fd00:ec2::23"):    "EKS IPv6 pod identity credentials endpoint",
}

var probeMetadataHosts = map[string]bool{
	"metadata.google.internal":   true,
	"metadata.goog":              true,
	"instance-data":              true,
	"instance-data.ec2.internal": true,
}

var (
	probeThisNetwork = netip.MustParsePrefix("0.0.0.0/8")
	probeSharedCGNAT = netip.MustParsePrefix("100.64.0.0/10")
	probeReservedV4  = netip.MustParsePrefix("240.0.0.0/4")
	probeSiteLocalV6 = netip.MustParsePrefix("fec0::/10")
	probeNAT64       = netip.MustParsePrefix("64:ff9b::/96")
)

// classifyProbeAddr sorts a destination address into the probe's policy
// buckets. IPv4-mapped and NAT64-embedded IPv4 addresses are judged by the
// IPv4 address they carry so they cannot smuggle a blocked target.
func classifyProbeAddr(addr netip.Addr) (probeAddrClass, string) {
	addr = addr.WithZone("").Unmap()
	if addr.Is6() && probeNAT64.Contains(addr) {
		b := addr.As16()
		addr = netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]})
	}
	if reason, ok := probeMetadataAddrs[addr]; ok {
		return probeAddrForbidden, reason
	}
	switch {
	case !addr.IsValid():
		return probeAddrForbidden, "invalid address"
	case addr.IsUnspecified() || probeThisNetwork.Contains(addr):
		return probeAddrForbidden, "unspecified/\"this network\" address"
	case addr.IsMulticast() || addr == netip.AddrFrom4([4]byte{255, 255, 255, 255}) || probeReservedV4.Contains(addr):
		return probeAddrForbidden, "multicast, broadcast, or reserved address"
	case addr.IsLoopback():
		return probeAddrLoopback, "loopback address"
	case addr.IsLinkLocalUnicast():
		return probeAddrPrivate, "link-local address"
	case addr.IsPrivate() || probeSharedCGNAT.Contains(addr) || probeSiteLocalV6.Contains(addr):
		return probeAddrPrivate, "private-network address"
	}
	return probeAddrPublic, ""
}

// checkProbeAddr applies the probe's network policy to one destination.
// Public and loopback destinations are allowed; private and link-local ones
// need allowPrivate; metadata and non-unicast destinations are always refused.
func checkProbeAddr(addr netip.Addr, allowPrivate bool) error {
	class, reason := classifyProbeAddr(addr)
	switch class {
	case probeAddrForbidden:
		return fmt.Errorf("%w: %s is a %s and is never probed", ErrProbeTargetBlocked, addr, reason)
	case probeAddrPrivate:
		if !allowPrivate {
			return fmt.Errorf("%w: %s is a %s; set %s=1 in the environment of the Blunt Code process to probe internal targets", ErrProbeTargetBlocked, addr, reason, PentestAllowPrivateEnv)
		}
	}
	return nil
}

// preflightProbeHost rejects targets whose literal address or current DNS
// answers violate the policy, so a refused target fails with a clear error
// before any connection is attempted. It is advisory: the dialer's control
// hook re-checks the address actually connected to, which is what defeats DNS
// rebinding. Lookup failures are left for the request itself to report.
func preflightProbeHost(ctx context.Context, host string, allowPrivate bool) error {
	name := strings.ToLower(strings.TrimSuffix(host, "."))
	if probeMetadataHosts[name] {
		return fmt.Errorf("%w: %s is a cloud metadata hostname and is never probed", ErrProbeTargetBlocked, host)
	}
	if addr, err := netip.ParseAddr(name); err == nil {
		return checkProbeAddr(addr, allowPrivate)
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupNetIP(lookupCtx, "ip", name)
	if err != nil {
		return nil
	}
	for _, addr := range addrs {
		if err := checkProbeAddr(addr, allowPrivate); err != nil {
			return fmt.Errorf("%s resolves to %s: %w", host, addr, err)
		}
	}
	return nil
}

// probeDialControl returns a net.Dialer Control hook that enforces the
// policy on the resolved address of every connection the probe opens,
// including redirects and follow-up path probes.
func probeDialControl(allowPrivate bool) func(network, address string, _ syscall.RawConn) error {
	return func(network, address string, _ syscall.RawConn) error {
		ap, err := netip.ParseAddrPort(address)
		if err != nil {
			return fmt.Errorf("%w: unparseable dial address %q", ErrProbeTargetBlocked, address)
		}
		return checkProbeAddr(ap.Addr(), allowPrivate)
	}
}
