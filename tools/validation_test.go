package tools

import (
	"testing"
)

func TestValidateIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		{"valid ipv4", "192.0.2.1", false},
		{"valid ipv6", "2001:db8::1", false},
		{"invalid ipv4 out of range", "192.0.2.300", true},
		{"invalid format", "not-an-ip", true},
		{"empty string", "", true},
		{"cidr notation rejected as single ip", "192.0.2.1/24", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIP(tt.ip, "test_ip")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIP(%q) error = %v, wantErr %v", tt.ip, err, tt.wantErr)
			}
		})
	}
}

func TestValidateIPv4(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		{"valid ipv4", "192.0.2.1", false},
		{"ipv6 rejected", "2001:db8::1", true},
		{"invalid string", "abc", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIPv4(tt.ip, "test_ip")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIPv4(%q) error = %v, wantErr %v", tt.ip, err, tt.wantErr)
			}
		})
	}
}

func TestValidateIPv6(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		{"valid ipv6", "2001:db8::1", false},
		{"ipv4 rejected", "192.0.2.1", true},
		{"invalid string", "abc", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIPv6(tt.ip, "test_ip")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIPv6(%q) error = %v, wantErr %v", tt.ip, err, tt.wantErr)
			}
		})
	}
}

func TestValidateSubnetPrefix(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		prefix  string
		wantErr bool
	}{
		{"valid ipv4 /24", "192.0.2.0", "24", false},
		{"valid ipv4 /32", "192.0.2.1", "32", false},
		{"valid ipv4 /0", "0.0.0.0", "0", false},
		{"invalid ipv4 prefix > 32", "192.0.2.0", "33", true},
		{"invalid ipv4 negative prefix", "192.0.2.0", "-1", true},
		{"valid ipv6 /64", "2001:db8::", "64", false},
		{"invalid ipv6 prefix > 128", "2001:db8::", "129", true},
		{"non-numeric prefix", "192.0.2.0", "twenty-four", true},
		{"invalid address", "invalid-ip", "24", true},
		{"empty address", "", "24", true},
		{"empty prefix", "192.0.2.0", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSubnetPrefix(tt.addr, tt.prefix)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSubnetPrefix(%q, %q) error = %v, wantErr %v", tt.addr, tt.prefix, err, tt.wantErr)
			}
		})
	}
}

func TestValidateMAC(t *testing.T) {
	tests := []struct {
		name    string
		mac     string
		wantErr bool
	}{
		{"empty allowed for optional", "", false},
		{"standard colon mac", "00:11:22:33:44:55", false},
		{"standard hyphen mac", "00-11-22-33-44-55", false},
		{"cisco dot mac", "0011.2233.4455", false},
		{"solidserver 7-octet dhcp mac", "01:00:11:22:33:44:55", false},
		{"invalid mac characters", "00:11:22:33:44:zz", true},
		{"invalid mac length", "00:11:22:33", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMAC(tt.mac, "mac")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMAC(%q) error = %v, wantErr %v", tt.mac, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDHCPMAC(t *testing.T) {
	tests := []struct {
		name    string
		mac     string
		wantErr bool
	}{
		{"solidserver 7-octet ethernet mac", "01:00:11:22:33:44:55", false},
		{"standard 6-octet colon mac", "00:11:22:33:44:55", false},
		{"empty required", "", true},
		{"invalid mac format", "invalid-mac", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDHCPMAC(tt.mac, "mac")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDHCPMAC(%q) error = %v, wantErr %v", tt.mac, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDNSRecordType(t *testing.T) {
	tests := []struct {
		name     string
		rrType   string
		wantType string
		wantErr  bool
	}{
		{"standard A uppercase", "A", "A", false},
		{"lowercase cname normalized to CNAME", "cname", "CNAME", false},
		{"valid AAAA", "AAAA", "AAAA", false},
		{"valid TXT", "TXT", "TXT", false},
		{"valid MX", "MX", "MX", false},
		{"valid PTR", "PTR", "PTR", false},
		{"invalid dns type", "UNKNOWN_RECORD", "", true},
		{"empty dns type", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateDNSRecordType(tt.rrType)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDNSRecordType(%q) error = %v, wantErr %v", tt.rrType, err, tt.wantErr)
			}
			if got != tt.wantType {
				t.Errorf("ValidateDNSRecordType(%q) = %q, want %q", tt.rrType, got, tt.wantType)
			}
		})
	}
}

func TestValidateDNSRecordValue(t *testing.T) {
	tests := []struct {
		name    string
		rrType  string
		value   string
		wantErr bool
	}{
		{"valid A record IPv4", "A", "192.0.2.1", false},
		{"invalid A record IPv6", "A", "2001:db8::1", true},
		{"invalid A record text", "A", "example.com", true},
		{"valid AAAA record IPv6", "AAAA", "2001:db8::1", false},
		{"invalid AAAA record IPv4", "AAAA", "192.0.2.1", true},
		{"valid CNAME host", "CNAME", "target.example.com", false},
		{"valid TXT string", "TXT", "v=spf1 ~all", false},
		{"empty value rejected", "A", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDNSRecordValue(tt.rrType, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDNSRecordValue(%q, %q) error = %v, wantErr %v", tt.rrType, tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestValidateVlanID(t *testing.T) {
	tests := []struct {
		name    string
		id      int32
		wantErr bool
	}{
		{"valid vlan 1", 1, false},
		{"valid vlan 100", 100, false},
		{"valid vlan 4094", 4094, false},
		{"invalid vlan 0", 0, true},
		{"invalid vlan negative", -10, true},
		{"invalid vlan 4095", 4095, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVlanID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVlanID(%d) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestEscapeWhereValue(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain string", "default", "default"},
		{"string with single quote", "corp's space", `corp\'s space`},
		{"string with sql injection attempt", `' OR '1'='1`, `\' OR \'1\'=\'1`},
		{"string with backslash and quote", `foo\bar'baz`, `foo\\bar\'baz`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapeWhereValue(tt.in)
			if got != tt.want {
				t.Errorf("EscapeWhereValue(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidateWhereClause(t *testing.T) {
	tests := []struct {
		name    string
		where   string
		wantErr bool
	}{
		{"empty where allowed", "", false},
		{"valid where clause with matched quotes", "vlan_name LIKE 'guest%'", false},
		{"valid where clause with multiple pairs", "domain_name='corp' AND vlan_id='10'", false},
		{"valid where clause with escaped quote", `vlan_name='O\'Reilly'`, false},
		{"unbalanced single quote", "vlan_name = 'guest", true},
		{"null byte rejected", "vlan_name = 'guest'\x00", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWhereClause(tt.where)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWhereClause(%q) error = %v, wantErr %v", tt.where, err, tt.wantErr)
			}
		})
	}
}
