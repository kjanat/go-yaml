// Copyright 2026 The go-yaml Project Contributors
// SPDX-License-Identifier: Apache-2.0

package libyaml

// Occurrence identifies the original node selected during construction.
// Node is neither copied nor dereferenced and may itself be an alias.
// Crossed contains alias occurrences entered since this load's root, outermost
// first. It excludes aliases supplied to [Node.LoadWithAliasContext].
// The slice is independent of constructor state; its nodes share the source
// graph and are meaningful only while that graph remains unchanged.
type Occurrence struct {
	Node    *Node
	Crossed []*Node
}

// OccurrenceTarget marks a value whose source is selected by [WithOccurrenceFunc]
// instead of constructed. The constructor recognizes addressable values whose
// address implements this interface. It does not call YAMLSelectionTarget or
// write the value; the callback receives its address and the source occurrence.
// Use a nonzero-sized type when distinguishing fields by their addresses.
type OccurrenceTarget interface {
	YAMLSelectionTarget()
}
