package tools

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

// Guardrails configures read-only restrictions and protected object rules.
type Guardrails struct {
	ReadOnly         bool
	ProtectedSpaces  []string
	ProtectedZones   []string
	ProtectedSubnets []string
}

// CheckReadOnly returns an error if the server is in read-only mode.
func (g *Guardrails) CheckReadOnly() error {
	if g != nil && g.ReadOnly {
		return errors.New("server is in read-only mode: mutating operations are disabled")
	}
	return nil
}

// CheckProtectedSpace returns an error if space matches any protected space.
func (g *Guardrails) CheckProtectedSpace(space string) error {
	if g == nil || space == "" || len(g.ProtectedSpaces) == 0 {
		return nil
	}
	for _, p := range g.ProtectedSpaces {
		if strings.EqualFold(strings.TrimSpace(p), strings.TrimSpace(space)) {
			return fmt.Errorf("cannot modify or delete protected space %q", space)
		}
	}
	return nil
}

// CheckProtectedZone returns an error if zone matches any protected DNS zone.
func (g *Guardrails) CheckProtectedZone(zone string) error {
	if g == nil || zone == "" || len(g.ProtectedZones) == 0 {
		return nil
	}
	trimmedZone := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(zone), "."))
	for _, p := range g.ProtectedZones {
		trimmedP := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(p), "."))
		if trimmedZone == trimmedP {
			return fmt.Errorf("cannot modify or delete protected DNS zone %q", zone)
		}
	}
	return nil
}

// overlappingProtectedSubnet reports the first protected subnet that the
// inclusive address range [start, end] overlaps, if any. A single-endpoint
// check is not enough: a range can start outside every protected subnet yet
// span one, so both endpoints and any protected subnet enclosed between them
// must be considered. This is the shared core behind both the range-creation
// guard and the resolve-before-enforce checks that delete or resize an existing
// object, whose own extent can enclose a smaller protected subnet a bare-address
// check would miss. Unparseable input is treated as no overlap (left to
// validation), matching CheckProtectedSubnet.
func (g *Guardrails) overlappingProtectedSubnet(start, end string) (string, bool) {
	if g == nil || len(g.ProtectedSubnets) == 0 {
		return "", false
	}
	lo, _ := netip.ParseAddr(strings.TrimSpace(start))
	hi, _ := netip.ParseAddr(strings.TrimSpace(end))
	if !lo.IsValid() || !hi.IsValid() {
		return "", false
	}
	if hi.Less(lo) {
		lo, hi = hi, lo
	}
	for _, p := range g.ProtectedSubnets {
		prot, err := netip.ParsePrefix(strings.TrimSpace(p))
		if err != nil {
			continue
		}
		first := prot.Masked().Addr()
		// Overlap iff an endpoint sits inside the protected subnet, or the
		// subnet's first address sits inside the range (subnet enclosed by it).
		if prot.Contains(lo) || prot.Contains(hi) || (lo.Compare(first) <= 0 && first.Compare(hi) <= 0) {
			return p, true
		}
	}
	return "", false
}

// CheckProtectedRange returns an error if the inclusive address range
// [start, end] overlaps any protected subnet, for a range-creation operation.
func (g *Guardrails) CheckProtectedRange(start, end string) error {
	if p, ok := g.overlappingProtectedSubnet(start, end); ok {
		return fmt.Errorf("cannot create a range %q-%q overlapping protected subnet %q", start, end, p)
	}
	return nil
}

// CheckProtectedSubnetCIDR runs the protected-subnet guard for an object given
// by a start address and prefix-length string, normalizing through canonicalCIDR
// so a non-canonical ("08") or family-invalid ("/128" on IPv4) prefix cannot
// make the underlying netip.ParsePrefix fail and the guard fall open. A prefix
// that cannot form a valid CIDR falls back to the bare-address containment
// check; its malformed prefix is rejected by input validation next.
func (g *Guardrails) CheckProtectedSubnetCIDR(address, prefix string) error {
	if cidr, ok := canonicalCIDR(address, prefix); ok {
		return g.CheckProtectedSubnet(cidr)
	}
	return g.CheckProtectedSubnet(address)
}

// CheckProtectedSubnet returns an error if subnet or IP matches or is contained within any protected subnet.
func (g *Guardrails) CheckProtectedSubnet(subnet string) error {
	if g == nil || subnet == "" || len(g.ProtectedSubnets) == 0 {
		return nil
	}
	trimmedSubnet := strings.TrimSpace(subnet)
	targetAddr, addrErr := netip.ParseAddr(trimmedSubnet)
	targetPrefix, prefixErr := netip.ParsePrefix(trimmedSubnet)

	for _, p := range g.ProtectedSubnets {
		trimmedP := strings.TrimSpace(p)
		if strings.EqualFold(trimmedP, trimmedSubnet) {
			return fmt.Errorf("cannot modify or delete protected subnet %q", subnet)
		}
		if protPrefix, err := netip.ParsePrefix(trimmedP); err == nil {
			if addrErr == nil && protPrefix.Contains(targetAddr) {
				return fmt.Errorf("cannot modify or delete address %q within protected subnet %q", subnet, p)
			}
			if prefixErr == nil && protPrefix.Overlaps(targetPrefix) {
				return fmt.Errorf("cannot modify or delete subnet %q overlapping protected subnet %q", subnet, p)
			}
		}
	}
	return nil
}
