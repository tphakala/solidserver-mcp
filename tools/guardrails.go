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
