package tools

import (
	"fmt"
	"maps"
	"net"
	"net/netip"
	"regexp"
	"slices"
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

var allowedDNSRecordTypesList string

func init() {
	types := slices.Collect(maps.Keys(allowedDNSRecordTypes))
	slices.Sort(types)
	allowedDNSRecordTypesList = strings.Join(types, ", ")
}

// dhcpMACRegex matches SolidServer DHCP MAC formats:
// 1) 7 octets: 01:xx:xx:xx:xx:xx:xx (type 01 for Ethernet + 6 byte MAC)
// 2) Standard 6 octets: xx:xx:xx:xx:xx:xx or xx-xx-xx-xx-xx-xx
var (
	dhcp7OctetMACRegex = regexp.MustCompile(`^[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){6}$`)
	dhcp6OctetMACRegex = regexp.MustCompile(`^[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}$`)
)

// ValidateRequiredString verifies that a required string parameter is not blank and contains no null bytes.
func ValidateRequiredString(val, paramName string) error {
	if strings.TrimSpace(val) == "" {
		return fmt.Errorf("%s parameter is required and cannot be empty", paramName)
	}
	if strings.ContainsRune(val, '\x00') {
		return fmt.Errorf("%s parameter cannot contain null bytes", paramName)
	}
	return nil
}

// ValidateOptionalString verifies that an optional string parameter contains no null bytes.
func ValidateOptionalString(val, paramName string) error {
	if val != "" && strings.ContainsRune(val, '\x00') {
		return fmt.Errorf("%s parameter cannot contain null bytes", paramName)
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
	if strings.ContainsRune(ipStr, '\x00') {
		return fmt.Errorf("%s cannot contain null bytes", paramName)
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ipStr))
	if err != nil || !addr.IsValid() {
		return fmt.Errorf("%s %q is not a valid IP address", paramName, ipStr)
	}
	return nil
}

// ValidateIPv4 verifies that the given string is a syntactically valid IPv4 address.
func ValidateIPv4(ipStr, paramName string) error {
	if err := ValidateIP(ipStr, paramName); err != nil {
		return err
	}
	addr, _ := netip.ParseAddr(strings.TrimSpace(ipStr))
	if !addr.Is4() {
		return fmt.Errorf("%s %q is not a valid IPv4 address", paramName, ipStr)
	}
	return nil
}

// ValidateIPv6 verifies that the given string is a syntactically valid IPv6 address.
func ValidateIPv6(ipStr, paramName string) error {
	if err := ValidateIP(ipStr, paramName); err != nil {
		return err
	}
	addr, _ := netip.ParseAddr(strings.TrimSpace(ipStr))
	if !addr.Is6() {
		return fmt.Errorf("%s %q is not a valid IPv6 address", paramName, ipStr)
	}
	return nil
}

const (
	maxDomainLength   = 253
	maxLabelLength    = 63
	maxPortNumber     = 65535
	maxCAAFlags       = 255
	srvFieldCount     = 4
	minCAAFieldCnt    = 3
	mxPrefAndExchange = 2
	mxExchangeOnly    = 1
)

func isValidLabelChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
}

func validateDomainLabel(label, paramName, domain string) error {
	if label == "" {
		return fmt.Errorf("%s %q contains empty label", paramName, domain)
	}
	if len(label) > maxLabelLength {
		return fmt.Errorf("%s label %q exceeds maximum length of 63 characters", paramName, label)
	}
	if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
		return fmt.Errorf("%s label %q cannot start or end with a hyphen", paramName, label)
	}
	for _, r := range label {
		if !isValidLabelChar(r) {
			return fmt.Errorf("%s label %q contains invalid character %q", paramName, label, r)
		}
	}
	return nil
}

// ValidateDomainName checks that a string is a valid FQDN or relative domain name.
func ValidateDomainName(domain, paramName string) error {
	trimmed := strings.TrimSpace(domain)
	if trimmed == "" {
		return fmt.Errorf("%s parameter is required and cannot be empty", paramName)
	}
	if strings.ContainsRune(trimmed, '\x00') {
		return fmt.Errorf("%s cannot contain null bytes", paramName)
	}
	d := strings.TrimSuffix(trimmed, ".")
	if d == "" {
		return fmt.Errorf("%s %q is not a valid domain name", paramName, domain)
	}
	if len(d) > maxDomainLength {
		return fmt.Errorf("%s %q exceeds maximum domain length of 253 characters", paramName, domain)
	}
	labels := strings.Split(d, ".")
	for _, label := range labels {
		if err := validateDomainLabel(label, paramName, domain); err != nil {
			return err
		}
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

	prefix, prefixErr := addr.Prefix(prefixInt)
	if prefixErr != nil || !prefix.IsValid() {
		return fmt.Errorf("invalid CIDR prefix %s/%d: %w", addr.String(), prefixInt, prefixErr)
	}

	return nil
}

// ValidateMAC verifies that a string is a valid MAC address.
func ValidateMAC(macStr, paramName string) error {
	if strings.TrimSpace(macStr) == "" {
		return nil
	}
	if strings.ContainsRune(macStr, '\x00') {
		return fmt.Errorf("%s cannot contain null bytes", paramName)
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
	if strings.ContainsRune(macStr, '\x00') {
		return fmt.Errorf("%s cannot contain null bytes", paramName)
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
	if strings.ContainsRune(rrType, '\x00') {
		return "", fmt.Errorf("type parameter cannot contain null bytes")
	}
	upper := strings.ToUpper(strings.TrimSpace(rrType))
	if _, ok := allowedDNSRecordTypes[upper]; !ok {
		return "", fmt.Errorf("unsupported DNS record type %q (allowed types: %s)", rrType, allowedDNSRecordTypesList)
	}
	return upper, nil
}

func validateMXRecordValue(value string) error {
	parts := strings.Fields(value)
	switch len(parts) {
	case mxPrefAndExchange:
		pref, err := strconv.Atoi(parts[0])
		if err != nil || pref < 0 || pref > maxPortNumber {
			return fmt.Errorf("DNS MX record preference %q must be an integer between 0 and 65535", parts[0])
		}
		return ValidateDomainName(parts[1], "DNS MX record exchange")
	case mxExchangeOnly:
		return ValidateDomainName(parts[0], "DNS MX record exchange")
	default:
		return fmt.Errorf("DNS MX record value %q is invalid (expected '[preference] exchange')", value)
	}
}

func validateSRVRecordValue(value string) error {
	parts := strings.Fields(value)
	if len(parts) != srvFieldCount {
		return fmt.Errorf("DNS SRV record value %q is invalid (expected 'priority weight port target')", value)
	}
	for i, fieldName := range []string{"priority", "weight", "port"} {
		num, err := strconv.Atoi(parts[i])
		if err != nil || num < 0 || num > maxPortNumber {
			return fmt.Errorf("DNS SRV record %s %q must be an integer between 0 and 65535", fieldName, parts[i])
		}
	}
	return ValidateDomainName(parts[3], "DNS SRV record target")
}

const maxCAATagLength = 15

func validateCAARecordValue(value string) error {
	parts := strings.Fields(value)
	if len(parts) < minCAAFieldCnt {
		return fmt.Errorf("DNS CAA record value %q is invalid (expected 'flags tag value')", value)
	}
	flags, err := strconv.Atoi(parts[0])
	if err != nil || flags < 0 || flags > maxCAAFlags {
		return fmt.Errorf("DNS CAA record flags %q must be an integer between 0 and 255", parts[0])
	}
	tag := parts[1]
	if tag == "" || len(tag) > maxCAATagLength {
		return fmt.Errorf("DNS CAA record tag %q must contain 1 to 15 ASCII letters or digits", tag)
	}
	for _, r := range tag {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return fmt.Errorf("DNS CAA record tag %q must contain only ASCII letters or digits", tag)
		}
	}
	return nil
}

// ValidateDNSRecordValue validates the record value based on its type.
func ValidateDNSRecordValue(rrType, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("value parameter is required and cannot be empty")
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("value parameter cannot contain null bytes")
	}
	upper := strings.ToUpper(strings.TrimSpace(rrType))
	switch upper {
	case TypeA:
		return ValidateIPv4(value, "DNS A record value")
	case TypeAAAA:
		return ValidateIPv6(value, "DNS AAAA record value")
	case TypeCNAME, TypePTR, TypeNS, TypeDNAME:
		return ValidateDomainName(value, fmt.Sprintf("DNS %s record target", upper))
	case TypeMX:
		return validateMXRecordValue(value)
	case TypeSRV:
		return validateSRVRecordValue(value)
	case TypeCAA:
		return validateCAARecordValue(value)
	}
	return nil
}

// DNS zone type constants, mirroring the appliance's dns_zone_add zone_type
// field (see the vendored model_dns_zone_add_input.go doc).
const (
	ZoneTypeMaster         = "master"
	ZoneTypeSlave          = "slave"
	ZoneTypeForward        = "forward"
	ZoneTypeStub           = "stub"
	ZoneTypeHint           = "hint"
	ZoneTypeDelegationOnly = "delegation-only"
)

// allowedDNSZoneTypes is the set of zone types SolidServer accepts, matched
// case-insensitively.
var allowedDNSZoneTypes = map[string]struct{}{
	ZoneTypeMaster:         {},
	ZoneTypeSlave:          {},
	ZoneTypeForward:        {},
	ZoneTypeStub:           {},
	ZoneTypeHint:           {},
	ZoneTypeDelegationOnly: {},
}

const allowedDNSZoneTypesList = ZoneTypeMaster + ", " + ZoneTypeSlave + ", " + ZoneTypeForward + ", " +
	ZoneTypeStub + ", " + ZoneTypeHint + ", " + ZoneTypeDelegationOnly

// ValidateDNSZoneType verifies the zone type is one SolidServer recognizes.
func ValidateDNSZoneType(zoneType string) error {
	if strings.TrimSpace(zoneType) == "" {
		return fmt.Errorf("type parameter is required (one of %s)", allowedDNSZoneTypesList)
	}
	if strings.ContainsRune(zoneType, '\x00') {
		return fmt.Errorf("type parameter cannot contain null bytes")
	}
	if _, ok := allowedDNSZoneTypes[strings.ToLower(strings.TrimSpace(zoneType))]; !ok {
		return fmt.Errorf("unsupported DNS zone type %q (allowed types: %s)", zoneType, allowedDNSZoneTypesList)
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
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(escaped, `'`, `\'`)
}

// ValidateWhereClause validates user-supplied WHERE clauses to prevent unbalanced quotes, unbalanced parentheses, or null bytes.
func ValidateWhereClause(where string) error {
	if where == "" {
		return nil
	}
	if strings.ContainsRune(where, '\x00') {
		return fmt.Errorf("where clause contains invalid null byte")
	}

	inSingleQuote, parens, err := scanWhereClauseTokens(where)
	if err != nil {
		return err
	}
	if inSingleQuote {
		return fmt.Errorf("where clause has unclosed single quote")
	}
	if parens != 0 {
		return fmt.Errorf("where clause has unbalanced parentheses")
	}
	return nil
}

func scanWhereClauseTokens(where string) (inSingleQuote bool, parens int, err error) {
	escaped := false
	for i := 0; i < len(where); i++ {
		ch := where[i]
		if escaped {
			escaped = false
			continue
		}
		switch ch {
		case '\\':
			escaped = true
		case '\'':
			inSingleQuote = !inSingleQuote
		case '(':
			if !inSingleQuote {
				parens++
			}
		case ')':
			if !inSingleQuote {
				parens--
				if parens < 0 {
					return inSingleQuote, parens, fmt.Errorf("where clause has unbalanced parentheses")
				}
			}
		}
	}
	return inSingleQuote, parens, nil
}
