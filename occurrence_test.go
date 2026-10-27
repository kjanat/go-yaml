// Copyright 2026 The go-yaml Project Contributors
// SPDX-License-Identifier: Apache-2.0

package yaml_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/kjanat/go-yaml/v4"
)

type occurrenceSelection struct{ token byte }

func (*occurrenceSelection) YAMLSelectionTarget() { panic("marker must not be called") }

func occurrenceDocument(t *testing.T, source string) *yaml.Node {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(source), &document); err != nil {
		t.Fatal(err)
	}
	return &document
}

func TestOccurrencePreservesAliasNode(t *testing.T) {
	document := occurrenceDocument(t, "base: &base [one, two]\nfield: *base\n")
	root := document.Content[0]
	alias := root.Content[3]
	var target struct{ Field occurrenceSelection }
	calls := 0
	err := root.Load(&target, yaml.WithOccurrenceFunc(func(dst yaml.OccurrenceTarget, selected yaml.Occurrence) error {
		calls++
		if dst != &target.Field || selected.Node != alias || len(selected.Crossed) != 0 {
			t.Errorf("selection = %p %+v, want original alias %p", dst, selected, alias)
		}
		return nil
	}))
	if err != nil || calls != 1 {
		t.Fatalf("load = %v, calls = %d", err, calls)
	}
	if root.Content[3] != alias || alias.Kind != yaml.AliasNode || alias.Alias != root.Content[1] {
		t.Fatal("source alias changed")
	}
}

func TestOccurrenceAliasAncestryAndSeeds(t *testing.T) {
	document := occurrenceDocument(t, "base: &base {first: one, second: two}\nparent: &parent {<<: *base}\nuse: *parent\n")
	root := document.Content[0]
	base, parent, outer := root.Content[1], root.Content[3], root.Content[5]
	merge := parent.Content[1]
	for _, seeded := range []bool{false, true} {
		t.Run(map[bool]string{false: "entered outer", true: "inherited outer"}[seeded], func(t *testing.T) {
			var target struct{ First, Second occurrenceSelection }
			node := outer
			var seeds []*yaml.Node
			wantCrossed := []*yaml.Node{outer, merge}
			if seeded {
				node, seeds, wantCrossed = parent, []*yaml.Node{outer}, []*yaml.Node{merge}
			}
			var calls []yaml.OccurrenceTarget
			err := node.LoadWithAliasContext(&target, seeds, yaml.WithOccurrenceFunc(func(dst yaml.OccurrenceTarget, selected yaml.Occurrence) error {
				index := len(calls)
				calls = append(calls, dst)
				if selected.Node != base.Content[index*2+1] || !reflect.DeepEqual(selected.Crossed, wantCrossed) {
					t.Errorf("selection %d = %+v, want ancestry %p", index, selected, wantCrossed)
				}
				// Mutating this snapshot must not corrupt the active alias path.
				selected.Crossed[0] = nil
				return nil
			}))
			if err != nil || !reflect.DeepEqual(calls, []yaml.OccurrenceTarget{&target.First, &target.Second}) {
				t.Fatalf("load = %v, targets = %v", err, calls)
			}
		})
	}
}

func TestOccurrenceMergeSelectionOrder(t *testing.T) {
	document := occurrenceDocument(t, "a: &a {inputs: first, outputs: output}\nb: &b {inputs: second, secrets: merged}\nuse: {<<: [*a, *b], secrets: explicit}\n")
	root := document.Content[0]
	a, use := root.Content[1], root.Content[5]
	var target struct{ Inputs, Outputs, Secrets occurrenceSelection }
	var calls []yaml.OccurrenceTarget
	var nodes []*yaml.Node
	err := use.Load(&target, yaml.WithV3Defaults(), yaml.WithOccurrenceFunc(func(dst yaml.OccurrenceTarget, selected yaml.Occurrence) error {
		calls = append(calls, dst)
		nodes = append(nodes, selected.Node)
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []yaml.OccurrenceTarget{&target.Secrets, &target.Inputs, &target.Outputs}) {
		t.Fatalf("selection order = %v", calls)
	}
	if !reflect.DeepEqual(nodes, []*yaml.Node{use.Content[3], a.Content[1], a.Content[3]}) {
		t.Fatalf("winning source nodes = %p", nodes)
	}
}

func TestOccurrenceInterceptsBeforeDispatch(t *testing.T) {
	document := occurrenceDocument(t, "null\n")
	null := document.Content[0]
	alias := &yaml.Node{Kind: yaml.AliasNode, Alias: null, Line: 2, Column: 3}
	for _, source := range []*yaml.Node{document, null, alias} {
		var target occurrenceSelection
		calls := 0
		err := source.Load(&target, yaml.WithOccurrenceFunc(func(dst yaml.OccurrenceTarget, selected yaml.Occurrence) error {
			calls++
			if dst != &target || selected.Node != source || len(selected.Crossed) != 0 {
				t.Errorf("selected = %+v, want source %p", selected, source)
			}
			return nil
		}))
		if err != nil || calls != 1 {
			t.Fatalf("kind %d: load = %v, calls = %d", source.Kind, err, calls)
		}
	}
}

func TestOccurrenceDuplicatePreemptsSelection(t *testing.T) {
	document := occurrenceDocument(t, "field: first\nfield: second\n")
	var target struct{ Field occurrenceSelection }
	calls := 0
	err := document.Load(&target, yaml.WithUniqueKeys(), yaml.WithOccurrenceFunc(func(yaml.OccurrenceTarget, yaml.Occurrence) error {
		calls++
		return nil
	}))
	if err == nil || !strings.Contains(err.Error(), "already defined") || calls != 0 {
		t.Fatalf("load = %v, calls = %d", err, calls)
	}
}

func TestOccurrenceCallbackErrorAborts(t *testing.T) {
	document := occurrenceDocument(t, "first: one\nsecond: two\n")
	var target struct{ First, Second occurrenceSelection }
	want := errors.New("selection stopped")
	calls := 0
	err := document.Load(&target, yaml.WithOccurrenceFunc(func(yaml.OccurrenceTarget, yaml.Occurrence) error {
		calls++
		return want
	}))
	var located *yaml.LoadError
	if !errors.Is(err, want) || !errors.As(err, &located) || calls != 1 {
		t.Fatalf("load = %v, calls = %d", err, calls)
	}
	source := document.Content[0].Content[1]
	if located.Stage != yaml.ConstructorStage || located.Mark.Line != source.Line || located.Mark.Column != source.Column {
		t.Fatalf("error location = %+v, want %d:%d", located, source.Line, source.Column)
	}
}

func TestOccurrenceRequiresCallback(t *testing.T) {
	source := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Line: 4, Column: 7}
	for _, options := range [][]yaml.Option{nil, {yaml.WithOccurrenceFunc(nil)}} {
		var target occurrenceSelection
		err := source.Load(&target, options...)
		var located *yaml.LoadError
		if !errors.As(err, &located) || !strings.Contains(err.Error(), "requires WithOccurrenceFunc") {
			t.Fatalf("load = %v, want missing callback error", err)
		}
		if located.Stage != yaml.ConstructorStage || located.Mark.Line != 4 || located.Mark.Column != 7 {
			t.Fatalf("error location = %+v", located)
		}
	}
}

type occurrenceCount struct{ aliases, constructions int }
type occurrenceCounts []occurrenceCount

func (*occurrenceCounts) CheckDepth(int, *yaml.DepthContext) error { return nil }

func (c *occurrenceCounts) CheckAlias(aliases, constructions int) error {
	*c = append(*c, occurrenceCount{aliases, constructions})
	return nil
}

func TestOccurrenceCountsWithoutWalkingPayload(t *testing.T) {
	document := occurrenceDocument(t, "anchor: &anchor {field: ["+strings.Repeat("payload, ", 2000)+"]}\nuse: *anchor\n")
	root := document.Content[0]
	anchor, alias := root.Content[1], root.Content[3]
	for _, tc := range []struct {
		name   string
		node   *yaml.Node
		seeds  []*yaml.Node
		counts occurrenceCounts
	}{
		{"plain", anchor, nil, occurrenceCounts{{0, 1}, {0, 2}, {0, 3}}},
		{"alias", alias, nil, occurrenceCounts{{0, 1}, {1, 2}, {2, 3}, {3, 4}}},
		{"seed", anchor, []*yaml.Node{alias}, occurrenceCounts{{1, 1}, {2, 2}, {3, 3}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var target struct{ Field occurrenceSelection }
			var counts occurrenceCounts
			calls := 0
			err := tc.node.LoadWithAliasContext(&target, tc.seeds, yaml.WithPlugin(&counts), yaml.WithOccurrenceFunc(func(yaml.OccurrenceTarget, yaml.Occurrence) error {
				calls++
				if !reflect.DeepEqual(counts, tc.counts) {
					t.Errorf("counts before callback = %v, want %v", counts, tc.counts)
				}
				return nil
			}))
			if err != nil || calls != 1 || !reflect.DeepEqual(counts, tc.counts) {
				t.Fatalf("load = %v, calls = %d, counts = %v", err, calls, counts)
			}
		})
	}
}

func TestOccurrenceKeepsInvalidMergeError(t *testing.T) {
	document := occurrenceDocument(t, "field: one\n<<: invalid\n")
	var target struct{ Field occurrenceSelection }
	calls := 0
	err := document.Load(&target, yaml.WithOccurrenceFunc(func(yaml.OccurrenceTarget, yaml.Occurrence) error {
		calls++
		return nil
	}))
	if err == nil || !strings.Contains(err.Error(), "map merge requires map") || calls != 1 {
		t.Fatalf("load = %v, calls = %d", err, calls)
	}
}
