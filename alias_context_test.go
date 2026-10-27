package yaml_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/kjanat/go-yaml/v4"
)

type aliasContextCounts struct{ aliases, constructions int }

func (*aliasContextCounts) CheckDepth(int, *yaml.DepthContext) error { return nil }

func (c *aliasContextCounts) CheckAlias(aliases, constructions int) error {
	c.aliases, c.constructions = aliases, constructions
	return nil
}

func TestNodeLoadWithAliasContext(t *testing.T) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte("anchor: &a {first: value, self: *a, last: ignored}\nuse: *a\n"), &document); err != nil {
		t.Fatal(err)
	}
	anchor := document.Content[0].Content[1]
	inner := anchor.Content[3]
	outer := document.Content[0].Content[3]
	for _, tc := range []struct {
		name   string
		active []*yaml.Node
		count  int
	}{
		{"active inner occurrence", []*yaml.Node{outer, inner}, 5},
		{"same anchor different occurrence", []*yaml.Node{outer}, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			counts := &aliasContextCounts{}
			got := map[string]any{"existing": "unchanged"}
			err := anchor.LoadWithAliasContext(&got, tc.active, yaml.WithPlugin(counts))
			var failure *yaml.LoadError
			if !errors.As(err, &failure) || failure.Stage != yaml.ComposerStage || !strings.Contains(failure.Message, "contains itself") {
				t.Fatalf("error = %v, want alias cycle", err)
			}
			if failure.Mark.Line != inner.Line || failure.Mark.Column != inner.Column {
				t.Fatalf("cycle position = %+v, want %d:%d", failure.Mark, inner.Line, inner.Column)
			}
			if counts.aliases != tc.count || counts.constructions != tc.count {
				t.Errorf("counts = %+v, want %d inherited alias constructions", counts, tc.count)
			}
			if !reflect.DeepEqual(got, map[string]any{"existing": "unchanged", "first": "value"}) {
				t.Errorf("partial value = %#v", got)
			}
		})
	}

	var selected struct{ First string }
	if err := anchor.LoadWithAliasContext(&selected, []*yaml.Node{outer, inner}); err != nil || selected.First != "value" {
		t.Fatalf("ignored recursive field: value=%+v error=%v", selected, err)
	}
	if inner.Alias != anchor || outer.Alias != anchor || inner.Kind != yaml.AliasNode {
		t.Fatal("alias context changed source nodes")
	}
}

func TestNodeLoadRejectsMalformedAliasContextBeforeWriting(t *testing.T) {
	root := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "new"}
	alias := &yaml.Node{Kind: yaml.AliasNode, Alias: root}
	for _, active := range [][]*yaml.Node{{nil}, {root}, {{Kind: yaml.AliasNode}}, {alias, alias}} {
		got := "original"
		if err := root.LoadWithAliasContext(&got, active); err == nil || got != "original" {
			t.Errorf("invalid context: value=%q error=%v", got, err)
		}
	}
}
