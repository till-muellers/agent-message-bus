package spaces

import "testing"

func TestMembershipLeadsJoined(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		m            Membership
		leads, joins bool
	}{
		{"leader", Membership{Role: RoleLeader}, true, true},
		{"member", Membership{Role: RoleMember}, false, true},
		{"admin joined", Membership{Role: RoleAdmin, Admin: true}, true, true},
		{"admin not joined", Membership{Admin: true}, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.m.Leads(); got != tc.leads {
				t.Fatalf("Leads() = %v, want %v", got, tc.leads)
			}
			if got := tc.m.Joined(); got != tc.joins {
				t.Fatalf("Joined() = %v, want %v", got, tc.joins)
			}
		})
	}
}
