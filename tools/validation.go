package tools

import (
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

// DNS Record Type constants.
const (
	TypeA      = "A"
	TypeAAAA   = "AAAA"
	TypeCNAME  = "CNAME"
	TypeMX     = "MX"
	TypeTXT    = "TXT"
	TypePTR    = "PTR"
	TypeSRV    = "SRV"
	TypeNS     = "NS"
	TypeSOA    = "SOA"
	TypeNAPTR  = "NAPTR"
	TypeCAA    = "CAA"
	TypeSPF    = "SPF"
	TypeLOC    = "LOC"
	TypeHINFO  = "HINFO"
	TypeRP     = "RP"
	TypeSSHFP  = "SSHFP"
	TypeTLSA   = "TLSA"
	TypeDS     = "DS"
	TypeDNSKEY = "DNSKEY"
	TypeDNAME  = "DNAME"
)

// Allowed uppercase DNS record types recognized by SolidServer.
var allowedDNSRecordTypes = map[string]struct{}{
	TypeA:      {},
	TypeAAAA:   {},
	TypeCNAME:  {},
	TypeMX:     {},
	TypeTXT:    {},
	TypePTR:    {},
	TypeSRV:    {},
	TypeNS:     {},
	TypeSOA:    {},
	TypeNAPTR:  {},
	TypeCAA:    {},
	TypeSPF:    {},
	TypeLOC:    {},
	TypeHINFO:  {},
	TypeRP:     {},
	TypeSSHFP:  {},
	TypeTLSA:   {},
	TypeDS:     {},
	TypeDNSKEY: {},
	TypeDNAME:  {},
}

// dhcpMACRegex matches SolidServer DHCP MAC formats:
// 1) 7 octets: 01:xx:xx:xx:xx:xx:xx (type 01 for Ethernet + 6 byte MAC)
// 2) Standard 6 octets: xx:xx:xx:xx:xx:xx or xx-xx-xx-xx-xx-xx
var (
	dhcp7OctetMACRegex = regexp.MustCompile(`^[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){6}$`)
	dhcp6OctetMACRegex = regexp.MustCompile(`^[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}$`)
)

// ValidateRequiredString verifies that a required string parameter is not blank.
func ValidateRequiredString(val, paramName string) error {
	if strings.TrimSpace(val) == "" {
		return fmt.Errorf("%s parameter is required and cannot be empty", paramName)
	}
	return nil
}

// ValidatePositiveInt32 verifies that an integer parameter is positive (> 0).
func ValidatePositiveInt32(val int32, paramName string) error {
	if val <= 0 {
		return fmt.Errorf("%s must be a positive integer, got %d", paramName, val)
	}
	return nil
}

// ValidateIP verifies that the given string is a syntactically valid IPv4 or IPv6 address.
func ValidateIP(ipStr, paramName string) error {
	if strings.TrimSpace(ipStr) == "" {
		return fmt.Errorf("%s parameter is required", paramName)
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ipStr))
	if err != nil || !addr.IsValid() {
		return fmt.Errorf("%s %q is not a valid IP address", paramName, ipStr)
	}
	return nil
}

// ValidateIPv4 verifies that the given string is a syntactically valid IPv4 address.
func ValidateIPv4(ipStr, paramName string) error {
	if strings.TrimSpace(ipStr) == "" {
		return fmt.Errorf("%s parameter is required", paramName)
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ipStr))
	if err != nil || !addr.IsValid() || !addr.Is4() {
		return fmt.Errorf("%s %q is not a valid IPv4 address", paramName, ipStr)
	}
	return nil
}

// ValidateIPv6 verifies that the given string is a syntactically valid IPv6 address.
func ValidateIPv6(ipStr, paramName string) error {
	if strings.TrimSpace(ipStr) == "" {
		return fmt.Errorf("%s parameter is required", paramName)
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ipStr))
	if err != nil || !addr.IsValid() || !addr.Is6() {
		return fmt.Errorf("%s %q is not a valid IPv6 address", paramName, ipStr)
	}
	return nil
}

// ValidateSubnetPrefix verifies both the start IP address and the prefix length.
func ValidateSubnetPrefix(addrStr, prefixStr string) error {
	if strings.TrimSpace(addrStr) == "" {
		return fmt.Errorf("address parameter is required")
	}
	if strings.TrimSpace(prefixStr) == "" {
		return fmt.Errorf("prefix parameter is required")
	}

	addr, err := netip.ParseAddr(strings.TrimSpace(addrStr))
	if err != nil || !addr.IsValid() {
		return fmt.Errorf("subnet address %q is not a valid IP address", addrStr)
	}

	prefixInt, err := strconv.Atoi(strings.TrimSpace(prefixStr))
	if err != nil {
		return fmt.Errorf("prefix %q must be an integer", prefixStr)
	}

	if addr.Is4() {
		if prefixInt < 0 || prefixInt > 32 {
			return fmt.Errorf("prefix %d is invalid for IPv4 (must be between 0 and 32)", prefixInt)
		}
	} else if addr.Is6() {
		if prefixInt < 0 || prefixInt > 128 {
			return fmt.Errorf("prefix %d is invalid for IPv6 (must be between 0 and 128)", prefixInt)
		}
	}

	// Validate combined CIDR notation
	cidrStr := fmt.Sprintf("%s/%d", addr.String(), prefixInt)
	_, err = netip.ParsePrefix(cidrStr)
	if err != nil {
		return fmt.Errorf("invalid CIDR prefix %q: %w", cidrStr, err)
	}

	return nil
}

// ValidateMAC verifies that a string is a valid MAC address.
func ValidateMAC(macStr, paramName string) error {
	if strings.TrimSpace(macStr) == "" {
		return nil
	}
	// Check if it matches 7-octet SolidServer format (01:xx:xx:xx:xx:xx:xx)
	if dhcp7OctetMACRegex.MatchString(macStr) {
		return nil
	}
	// Check standard IEEE 802 MAC format
	_, err := net.ParseMAC(macStr)
	if err != nil {
		return fmt.Errorf("%s %q is not a valid MAC address (expected format e.g. '00:11:22:33:44:55' or '01:00:11:22:33:44:55')", paramName, macStr)
	}
	return nil
}

// ValidateDHCPMAC verifies a MAC address for DHCP static reservation.
func ValidateDHCPMAC(macStr, paramName string) error {
	if strings.TrimSpace(macStr) == "" {
		return fmt.Errorf("%s parameter is required", paramName)
	}
	trimmed := strings.TrimSpace(macStr)
	if dhcp7OctetMACRegex.MatchString(trimmed) || dhcp6OctetMACRegex.MatchString(trimmed) {
		return nil
	}
	// Check standard parse
	_, err := net.ParseMAC(trimmed)
	if err != nil {
		return fmt.Errorf("%s %q is not a valid DHCP MAC address (expected format '01:00:11:22:33:44:55' or '00:11:22:33:44:55')", paramName, macStr)
	}
	return nil
}

// ValidateDNSRecordType verifies and normalizes the DNS record type.
func ValidateDNSRecordType(rrType string) (string, error) {
	if strings.TrimSpace(rrType) == "" {
		return "", fmt.Errorf("type parameter is required")
	}
	upper := strings.ToUpper(strings.TrimSpace(rrType))
	if _, ok := allowedDNSRecordTypes[upper]; !ok {
		return "", fmt.Errorf("unsupported DNS record type %q (allowed types: A, AAAA, CNAME, MX, TXT, PTR, SRV, NS, SOA, CAA, NAPTR, SPF)", rrType)
	}
	return upper, nil
}

// ValidateDNSRecordValue validates the record value based on its type.
func ValidateDNSRecordValue(rrType, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("value parameter is required and cannot be empty")
	}
	upper := strings.ToUpper(strings.TrimSpace(rrType))
	switch upper {
	case TypeA:
		if err := ValidateIPv4(value, "DNS A record value"); err != nil {
			return err
		}
	case TypeAAAA:
		if err := ValidateIPv6(value, "DNS AAAA record value"); err != nil {
			return err
		}
	}
	return nil
}

// ValidateTTL verifies that the DNS TTL is non-negative.
func ValidateTTL(ttl int32) error {
	if ttl < 0 {
		return fmt.Errorf("ttl must be non-negative, got %d", ttl)
	}
	return nil
}

// ValidateVlanID verifies that a VLAN ID is in the standard valid range [1, 4094].
func ValidateVlanID(vlanID int32) error {
	if vlanID < 1 || vlanID > 4094 {
		return fmt.Errorf("vlan_id must be between 1 and 4094, got %d", vlanID)
	}
	return nil
}

// EscapeWhereValue escapes single quotes and backslashes in user values before interpolating into WHERE clauses.
func EscapeWhereValue(s string) string {
	// Escape backslashes first, then single quotes
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(escaped, `'`, `\'`)
}

// ValidateWhereClause validates user-supplied WHERE clauses to prevent unbalanced quotes or null bytes.
func ValidateWhereClause(where string) error {
	if where == "" {
		return nil
	}
	if strings.ContainsRune(where, '\x00') {
		return fmt.Errorf("where clause contains invalid null byte")
	}

	// Check quote balance (ignoring escaped quotes)
	inSingleQuote := false
	escaped := false
	for i := 0; i < len(where); i++ {
		ch := where[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '\'' {
			inSingleQuote = !inSingleQuote
		}
	}
	if inSingleQuote {
		return fmt.Errorf("where clause has unclosed single quote")
	}

	return nil
}
