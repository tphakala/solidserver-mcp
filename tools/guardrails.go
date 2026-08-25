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

// CheckProtectedRange returns an error if the inclusive address range
// [start, end] overlaps any protected subnet. A single-endpoint check is not
// enough for a range: a range can start outside every protected subnet yet hand
// out addresses inside one, so both endpoints and any protected subnet enclosed
// between them must be considered. Unparseable input is left to validation
// (returns nil), matching CheckProtectedSubnet.
func (g *Guardrails) CheckProtectedRange(start, end string) error {
	if g == nil || len(g.ProtectedSubnets) == 0 {
		return nil
	}
	lo, _ := netip.ParseAddr(strings.TrimSpace(start))
	hi, _ := netip.ParseAddr(strings.TrimSpace(end))
	if !lo.IsValid() || !hi.IsValid() {
		// Unparseable input is left to validation.
		return nil
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
			return fmt.Errorf("cannot create a range %q-%q overlapping protected subnet %q", start, end, p)
		}
	}
	return nil
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
