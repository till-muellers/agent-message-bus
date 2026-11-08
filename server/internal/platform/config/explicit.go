package config

import (
	"fmt"
	"strconv"
	"strings"
)

const requireExplicitVar = "AMB_REQUIRE_EXPLICIT"

// requireExplicit enforces AMB_REQUIRE_EXPLICIT: when it is true, AMB_ADDR, AMB_WEB_ADDR,
// AMB_DB_PATH and AMB_ADMIN_TOKEN must all be given, so no command can fall back to the default
// data dir or ports. Unset or empty leaves the defaults in force; a value that is not a boolean
// fails closed.
func requireExplicit(lookup func(string) (string, bool)) error {
	raw, _ := lookup(requireExplicitVar)
	if raw == "" {
		return nil
	}
	on, err := strconv.ParseBool(raw)
	if err != nil {
		return fmt.Errorf("%s: %q is not a boolean (use 1 or 0)", requireExplicitVar, raw)
	}
	if !on {
		return nil
	}
	if missing := missingExplicit(lookup); len(missing) > 0 {
		return fmt.Errorf("%s is set: %s missing; set all four so the default data dir stays untouched",
			requireExplicitVar, strings.Join(missing, ", "))
	}
	return nil
}

// missingExplicit lists the demanded variables that are not given. An empty AMB_WEB_ADDR counts
// as given (the GUI is then off); for the others an empty value falls back to a default, so it
// counts as missing.
func missingExplicit(lookup func(string) (string, bool)) []string {
	var missing []string
	for _, name := range []string{"AMB_ADDR", "AMB_WEB_ADDR", "AMB_DB_PATH", "AMB_ADMIN_TOKEN"} {
		v, ok := lookup(name)
		if !ok || (v == "" && name != "AMB_WEB_ADDR") {
			missing = append(missing, name)
		}
	}
	return missing
}
