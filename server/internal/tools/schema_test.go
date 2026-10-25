package tools

import (
	"fmt"
	"strings"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/domain"
)

func TestPropertyDescriptionsMatchDomainLimits(t *testing.T) {
	cases := []struct {
		name string
		prop map[string]any
		want string
	}{
		{"body", bodyProp(), fmt.Sprintf("1..%d bytes", domain.MaxBodyBytes)},
		{"title", titleProp(), fmt.Sprintf("1..%d characters", domain.MaxTitleChars)},
		{"space name", spaceProp(), domain.NamePattern},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			desc, _ := c.prop["description"].(string)
			if !strings.Contains(desc, c.want) {
				t.Fatalf("description %q does not contain %q", desc, c.want)
			}
		})
	}
}
