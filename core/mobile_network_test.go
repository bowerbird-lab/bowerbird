package core

import "testing"

func TestNetworkInterfaceSelected(t *testing.T) {
	tests := []struct {
		name    string
		allowed []string
		iface   string
		netType string
		ips     []string
		want    bool
	}{
		{
			name:    "nil defaults to LAN",
			allowed: nil,
			iface:   "en0",
			netType: "lan",
			ips:     []string{"192.168.1.10"},
			want:    true,
		},
		{
			name:    "nil leaves Tailscale off",
			allowed: nil,
			iface:   "utun3",
			netType: "tailscale",
			ips:     []string{"100.64.0.2"},
			want:    false,
		},
		{
			name:    "explicit empty selection disables LAN",
			allowed: []string{},
			iface:   "en0",
			netType: "lan",
			ips:     []string{"192.168.1.10"},
			want:    false,
		},
		{
			name:    "selected by interface id",
			allowed: []string{"utun3"},
			iface:   "utun3",
			netType: "tailscale",
			ips:     []string{"100.64.0.2"},
			want:    true,
		},
		{
			name:    "selected by network type",
			allowed: []string{"tailscale"},
			iface:   "utun3",
			netType: "tailscale",
			ips:     []string{"100.64.0.2"},
			want:    true,
		},
		{
			name:    "selected by address",
			allowed: []string{"192.168.1.10"},
			iface:   "en0",
			netType: "lan",
			ips:     []string{"192.168.1.10"},
			want:    true,
		},
		{
			name:    "blank entries are ignored",
			allowed: []string{"  ", " en0 "},
			iface:   "en0",
			netType: "lan",
			ips:     []string{"192.168.1.10"},
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := networkInterfaceSelected(tt.allowed, tt.iface, tt.netType, tt.ips); got != tt.want {
				t.Fatalf("networkInterfaceSelected(%v, %q, %q, %v) = %t, want %t", tt.allowed, tt.iface, tt.netType, tt.ips, got, tt.want)
			}
		})
	}
}

func TestNormalizeAllowedNetworks(t *testing.T) {
	if got := normalizeAllowedNetworks(nil); got != nil {
		t.Fatalf("nil selection should stay nil, got %#v", got)
	}
	got := normalizeAllowedNetworks([]string{" en0 ", "", "en0", "utun3", "utun3"})
	if len(got) != 2 || got[0] != "en0" || got[1] != "utun3" {
		t.Fatalf("normalizeAllowedNetworks = %#v, want [en0 utun3]", got)
	}
	empty := normalizeAllowedNetworks([]string{})
	if empty == nil || len(empty) != 0 {
		t.Fatalf("empty selection must remain explicit and non-nil, got %#v", empty)
	}
}

func TestAllowedNetworksEmptySelectionPersists(t *testing.T) {
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())

	store, err := NewUserDataStore()
	if err != nil {
		t.Fatalf("NewUserDataStore: %v", err)
	}
	settings := store.GetMobileSettings()
	if settings.AllowedNetworks != nil {
		t.Fatalf("new settings should use the default nil selection, got %#v", settings.AllowedNetworks)
	}
	settings.AllowedNetworks = []string{}
	if err := store.SetMobileSettings(settings); err != nil {
		t.Fatalf("SetMobileSettings: %v", err)
	}

	reopened, err := NewUserDataStore()
	if err != nil {
		t.Fatalf("reopen UserDataStore: %v", err)
	}
	got := reopened.GetMobileSettings().AllowedNetworks
	if got == nil {
		t.Fatal("empty allowedNetworks was written as null/omitted; all-disabled selection was lost")
	}
	if len(got) != 0 {
		t.Fatalf("expected no allowed networks, got %#v", got)
	}
}

func TestDefaultAndEmptyNetworkSelections(t *testing.T) {
	for _, network := range GetAvailableNetworkInterfaces(nil) {
		want := network.Type == "lan"
		if network.Selected != want {
			t.Fatalf("default selection for %s (%s) = %t, want %t", network.ID, network.Type, network.Selected, want)
		}
	}
	for _, network := range GetAvailableNetworkInterfaces([]string{}) {
		if network.Selected {
			t.Fatalf("explicit empty selection still exposed %s (%s)", network.ID, network.Type)
		}
	}
	if ips := GetFilteredLocalIPs([]string{}); len(ips) != 0 {
		t.Fatalf("explicit empty selection leaked candidate IPs: %v", ips)
	}
}
