package tools

import (
	"strings"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/domain"
)

// creatingNames lists the parameters that create a name; only they state the name rule.
var creatingNames = map[string]bool{
	"bus_create_space.space":           true,
	"bus_create_topic.topic":           true,
	"bus_create_blackboard.blackboard": true,
	"bus_mint_leader.name":             true,
	"bus_mint_admin.name":              true,
	"bus_invite_member.name":           true,
}

func TestNameRuleIsStatedOnlyWhereANameIsCreated(t *testing.T) {
	t.Parallel()
	found := map[string]bool{}
	for _, m := range Metadata() {
		props, _ := m.Schema["properties"].(map[string]any)
		for name, p := range props {
			desc, _ := p.(map[string]any)["description"].(string)
			if strings.Contains(desc, domain.NamePattern) {
				found[m.Name+"."+name] = true
			}
		}
	}
	for key := range creatingNames {
		if !found[key] {
			t.Errorf("%s lacks the name rule", key)
		}
	}
	for key := range found {
		if !creatingNames[key] {
			t.Errorf("%s states the name rule but names an existing name", key)
		}
	}
}

func TestExistingNamePropertiesAreShort(t *testing.T) {
	t.Parallel()
	for name, prop := range map[string]map[string]any{
		"space": spaceProp(), "topic": topicProp(), "blackboard": boardProp(),
	} {
		want := strings.ToUpper(name[:1]) + name[1:] + " name."
		if got := prop["description"]; got != want {
			t.Errorf("%s property reads %q, want %q", name, got, want)
		}
	}
}
