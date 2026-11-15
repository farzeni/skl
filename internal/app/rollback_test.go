package app

import (
	"testing"

	"github.com/farzeni/skl/internal/config"
	"github.com/farzeni/skl/internal/state"
)

// docs/guide/rollback.md "Go back": "Going back over an skl import gives you your
// folder back", and "A rollback is itself a new generation, so you can undo it with
// another skl rollback". The oldest generation that handed a folder over decides.
func TestHandovers(t *testing.T) {
	x := state.Copy{Name: "x", Target: config.Claude, Skill: "you/x"}
	xAgents := state.Copy{Name: "x", Target: config.Agents, Skill: "you/x"}
	y := state.Copy{Name: "y", Target: config.Claude, Skill: "you/y"}
	tests := []struct {
		name             string
		gens             []state.Generation
		release, readopt []state.Copy
	}{
		{"nothing handed over", []state.Generation{{Command: "use review"}}, nil, nil},
		{"over an import", []state.Generation{{Adopted: []state.Copy{x}}, {Command: "add review"}}, []state.Copy{x}, nil},
		{"over the rollback that gave it back", []state.Generation{{Released: []state.Copy{x}}}, nil, []state.Copy{x}},
		{"over both, the import first", []state.Generation{{Adopted: []state.Copy{x}}, {Released: []state.Copy{x}}}, []state.Copy{x}, nil},
		{"over both, the release first", []state.Generation{{Released: []state.Copy{x}}, {Adopted: []state.Copy{x}}}, nil, []state.Copy{x}},
		{"each target on its own", []state.Generation{{Adopted: []state.Copy{xAgents}}, {Adopted: []state.Copy{y}}}, []state.Copy{xAgents, y}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := handoversOf(tt.gens)
			if !sameCopies(h.release, tt.release) || !sameCopies(h.adopt, tt.readopt) {
				t.Errorf("handovers = %+v, want release %+v, adopt %+v", h, tt.release, tt.readopt)
			}
		})
	}
}
