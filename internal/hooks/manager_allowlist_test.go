package hooks

import "testing"

// TestAllowed_TruthTable exercises every row of the allowlist truth table
// from PHASE_4_PLAN.md §3.A.2. The matrix is:
//
//	Allowlist     | AllowSelfInv | Behavior
//	nil/empty     | true         | all commands pass; self-invoke also passes
//	nil/empty     | false        | all commands pass EXCEPT self-invoke
//	non-empty     | true         | only allowlist matches; self-invoke must also be allowlisted
//	non-empty     | false        | only allowlist matches AND no self-invoke (allowlist match cannot rescue self-invoke)
//
// Implementation note: when allowlist is non-empty, the allowlist is the
// primary gate. AllowSelfInv=true does NOT auto-allow self-invoke — it only
// disables the default self-invoke deny. To let self-invoke through with a
// non-empty allowlist, users must add a pattern like "^clue-code " explicitly.
// This is the safer interpretation: "explicit allowlist always wins".
func TestAllowed_TruthTable(t *testing.T) {
	cases := []struct {
		name         string
		allowlist    []string
		allowSelfInv bool
		command      string
		want         bool
	}{
		// Row 1: empty allowlist + AllowSelfInv=true → everything passes.
		{name: "row1_empty_allow_self_arbitrary", allowlist: nil, allowSelfInv: true, command: "echo hello", want: true},
		{name: "row1_empty_allow_self_self", allowlist: nil, allowSelfInv: true, command: "clue-code skill ralph", want: true},

		// Row 2: empty allowlist + AllowSelfInv=false → everything EXCEPT self-invoke.
		{name: "row2_empty_deny_self_arbitrary", allowlist: nil, allowSelfInv: false, command: "echo hello", want: true},
		{name: "row2_empty_deny_self_self", allowlist: nil, allowSelfInv: false, command: "clue-code skill ralph", want: false},
		{name: "row2_empty_deny_self_self_path", allowlist: nil, allowSelfInv: false, command: "/usr/local/bin/clue-code state list-active", want: false},

		// Row 3: non-empty allowlist + AllowSelfInv=true → allowlist is the gate.
		{name: "row3_list_allow_self_match", allowlist: []string{"^echo "}, allowSelfInv: true, command: "echo hello", want: true},
		{name: "row3_list_allow_self_no_match", allowlist: []string{"^echo "}, allowSelfInv: true, command: "rm -rf /", want: false},
		{name: "row3_list_allow_self_self_not_allowlisted", allowlist: []string{"^echo "}, allowSelfInv: true, command: "clue-code skill ralph", want: false},
		{name: "row3_list_allow_self_self_allowlisted", allowlist: []string{"^clue-code "}, allowSelfInv: true, command: "clue-code skill ralph", want: true},

		// Row 4: non-empty allowlist + AllowSelfInv=false → only allowlist AND no self-invoke.
		{name: "row4_list_deny_self_match", allowlist: []string{"^echo ", "^cat "}, allowSelfInv: false, command: "echo hello", want: true},
		{name: "row4_list_deny_self_no_match", allowlist: []string{"^echo "}, allowSelfInv: false, command: "rm -rf /", want: false},
		{name: "row4_list_deny_self_self_explicit", allowlist: []string{"^clue-code "}, allowSelfInv: false, command: "clue-code skill ralph", want: false},
		{name: "row4_list_deny_self_self_path_explicit", allowlist: []string{"clue-code"}, allowSelfInv: false, command: "/opt/bin/clue-code state list-active", want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := &Manager{cfg: &Config{Allowlist: c.allowlist, AllowSelfInv: c.allowSelfInv}}
			got := m.allowed(c.command)
			if got != c.want {
				t.Errorf("allowed(%q) with Allowlist=%v AllowSelfInv=%v: got %v, want %v",
					c.command, c.allowlist, c.allowSelfInv, got, c.want)
			}
		})
	}
}

// TestIsSelfInvoke covers the path-prefix and bare-name detection edges.
func TestIsSelfInvoke(t *testing.T) {
	cases := []struct {
		command string
		want    bool
	}{
		{"clue-code", true},                          // bare name
		{"clue-code state list-active", true},        // bare with args
		{"  clue-code skill ralph  ", true},          // surrounding whitespace
		{"/usr/local/bin/clue-code version", true},   // absolute path with space-suffix
		{"./clue-code doctor", true},                 // relative path with space-suffix
		{"sh -c 'clue-code skill autopilot'", false}, // shell wraps it; not at start of trimmed line
		{"echo clue-code", false},                    // mention but not invocation
		{"clue-code-other-tool", true},               // false-positive (acceptable: deny-by-default)
		{"", false},                                  // empty
		{"sleep 60", false},                          // unrelated
	}
	for _, c := range cases {
		t.Run(c.command, func(t *testing.T) {
			got := isSelfInvoke(c.command)
			if got != c.want {
				t.Errorf("isSelfInvoke(%q) = %v, want %v", c.command, got, c.want)
			}
		})
	}
}
