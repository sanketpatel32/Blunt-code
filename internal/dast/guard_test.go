package dast

import (
	"errors"
	"net/netip"
	"strings"
	"testing"
)

func TestClassifyProbeAddr(t *testing.T) {
	cases := []struct {
		addr string
		want probeAddrClass
	}{
		{"93.184.216.34", probeAddrPublic},
		{"2606:4700::1111", probeAddrPublic},
		{"127.0.0.1", probeAddrLoopback},
		{"127.10.0.5", probeAddrLoopback},
		{"::1", probeAddrLoopback},
		{"::ffff:127.0.0.1", probeAddrLoopback},
		{"10.0.0.1", probeAddrPrivate},
		{"172.16.5.4", probeAddrPrivate},
		{"192.168.1.1", probeAddrPrivate},
		{"100.64.0.1", probeAddrPrivate},
		{"169.254.10.10", probeAddrPrivate},
		{"fe80::1", probeAddrPrivate},
		{"fe80::1%eth0", probeAddrPrivate},
		{"fd12:3456::1", probeAddrPrivate},
		{"::ffff:192.168.1.1", probeAddrPrivate},
		{"64:ff9b::a00:1", probeAddrPrivate},
		{"169.254.169.254", probeAddrForbidden},
		{"::ffff:169.254.169.254", probeAddrForbidden},
		{"64:ff9b::a9fe:a9fe", probeAddrForbidden},
		{"169.254.170.2", probeAddrForbidden},
		{"100.100.100.200", probeAddrForbidden},
		{"fd00:ec2::254", probeAddrForbidden},
		{"0.0.0.0", probeAddrForbidden},
		{"0.1.2.3", probeAddrForbidden},
		{"::", probeAddrForbidden},
		{"224.0.0.1", probeAddrForbidden},
		{"ff02::1", probeAddrForbidden},
		{"255.255.255.255", probeAddrForbidden},
		{"240.0.0.1", probeAddrForbidden},
	}
	for _, tc := range cases {
		got, _ := classifyProbeAddr(netip.MustParseAddr(tc.addr))
		if got != tc.want {
			t.Errorf("classifyProbeAddr(%s) = %d, want %d", tc.addr, got, tc.want)
		}
	}
}

func TestCheckProbeAddrPolicy(t *testing.T) {
	for _, allow := range []bool{false, true} {
		if err := checkProbeAddr(netip.MustParseAddr("169.254.169.254"), allow); !errors.Is(err, ErrProbeTargetBlocked) {
			t.Errorf("metadata endpoint must be blocked with allowPrivate=%v, got %v", allow, err)
		}
		if err := checkProbeAddr(netip.MustParseAddr("127.0.0.1"), allow); err != nil {
			t.Errorf("loopback must be allowed with allowPrivate=%v, got %v", allow, err)
		}
		if err := checkProbeAddr(netip.MustParseAddr("8.8.8.8"), allow); err != nil {
			t.Errorf("public must be allowed with allowPrivate=%v, got %v", allow, err)
		}
	}
	if err := checkProbeAddr(netip.MustParseAddr("192.168.1.1"), false); !errors.Is(err, ErrProbeTargetBlocked) || !strings.Contains(err.Error(), PentestAllowPrivateEnv) {
		t.Errorf("private target must be blocked by default and name the opt-in, got %v", err)
	}
	if err := checkProbeAddr(netip.MustParseAddr("192.168.1.1"), true); err != nil {
		t.Errorf("private target must be allowed when opted in, got %v", err)
	}
}

// TestProbeDialControlEnforcesResolvedAddress pins the dial-time check that
// defeats DNS rebinding: whatever the hostname resolved to, the address being
// connected to is re-judged.
func TestProbeDialControlEnforcesResolvedAddress(t *testing.T) {
	control := probeDialControl(false)
	if err := control("tcp4", "169.254.169.254:80", nil); !errors.Is(err, ErrProbeTargetBlocked) {
		t.Fatalf("metadata dial must be blocked, got %v", err)
	}
	if err := control("tcp4", "10.1.2.3:443", nil); !errors.Is(err, ErrProbeTargetBlocked) {
		t.Fatalf("private dial must be blocked by default, got %v", err)
	}
	if err := control("tcp6", "[::1]:8080", nil); err != nil {
		t.Fatalf("loopback dial must be allowed, got %v", err)
	}
	if err := probeDialControl(true)("tcp4", "169.254.169.254:80", nil); !errors.Is(err, ErrProbeTargetBlocked) {
		t.Fatalf("metadata dial must be blocked even when private targets are allowed, got %v", err)
	}
}
