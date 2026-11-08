package config

import (
	"strings"
	"testing"
)

func TestRequireExplicit(t *testing.T) {
	t.Parallel()
	token := strings.Repeat("a", 32)
	all := map[string]string{"AMB_ADDR": "127.0.0.1:9", "AMB_WEB_ADDR": "off", "AMB_DB_PATH": "x.db", "AMB_ADMIN_TOKEN": token}
	tests := []struct {
		name    string
		guard   string
		drop    []string
		set     map[string]string
		wantErr string
	}{
		{name: "guard unset allows the defaults", guard: "", drop: []string{"AMB_ADDR", "AMB_WEB_ADDR", "AMB_DB_PATH", "AMB_ADMIN_TOKEN"}},
		{name: "guard 0 allows the defaults", guard: "0", drop: []string{"AMB_ADDR", "AMB_WEB_ADDR", "AMB_DB_PATH", "AMB_ADMIN_TOKEN"}},
		{name: "guard on with all four set", guard: "1"},
		{name: "guard on accepts an empty AMB_WEB_ADDR", guard: "1", set: map[string]string{"AMB_WEB_ADDR": ""}},
		{
			name: "guard on names every missing variable", guard: "1", drop: []string{"AMB_ADDR", "AMB_WEB_ADDR", "AMB_DB_PATH", "AMB_ADMIN_TOKEN"},
			wantErr: "AMB_REQUIRE_EXPLICIT is set: AMB_ADDR, AMB_WEB_ADDR, AMB_DB_PATH, AMB_ADMIN_TOKEN missing; set all four so the default data dir stays untouched",
		},
		{
			name: "guard on names only the missing one", guard: "true", drop: []string{"AMB_DB_PATH"},
			wantErr: "AMB_REQUIRE_EXPLICIT is set: AMB_DB_PATH missing; set all four so the default data dir stays untouched",
		},
		{
			name: "guard on treats empty AMB_DB_PATH as missing", guard: "1", set: map[string]string{"AMB_DB_PATH": ""},
			wantErr: "AMB_REQUIRE_EXPLICIT is set: AMB_DB_PATH missing; set all four so the default data dir stays untouched",
		},
		{
			name: "guard on treats empty AMB_ADDR as missing", guard: "1", set: map[string]string{"AMB_ADDR": ""},
			wantErr: "AMB_REQUIRE_EXPLICIT is set: AMB_ADDR missing; set all four so the default data dir stays untouched",
		},
		{
			name: "guard on treats empty AMB_ADMIN_TOKEN as missing", guard: "1", set: map[string]string{"AMB_ADMIN_TOKEN": ""},
			wantErr: "AMB_REQUIRE_EXPLICIT is set: AMB_ADMIN_TOKEN missing; set all four so the default data dir stays untouched",
		},
		{name: "unparseable guard fails closed", guard: "maybe", wantErr: `AMB_REQUIRE_EXPLICIT: "maybe" is not a boolean (use 1 or 0)`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := map[string]string{}
			for k, v := range all {
				env[k] = v
			}
			for _, k := range tc.drop {
				delete(env, k)
			}
			for k, v := range tc.set {
				env[k] = v
			}
			if tc.guard != "" {
				env["AMB_REQUIRE_EXPLICIT"] = tc.guard
			}
			env["XDG_DATA_HOME"] = "X"
			_, err := load(lookupIn(env), "linux", func() (string, error) { return "H", nil })
			if tc.wantErr == "" && err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if tc.wantErr != "" && (err == nil || err.Error() != tc.wantErr) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}
