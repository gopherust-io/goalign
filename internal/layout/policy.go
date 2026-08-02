package layout

import "fmt"

// Policy selects how Suggest reorders fields.
type Policy string

const (
	// PolicyAtomics is atomics-first then density packing (default).
	PolicyAtomics Policy = "atomics"
	// PolicyDensity is pure fieldalignment-style density sort.
	PolicyDensity Policy = "density"
	// PolicyStable density-sorts but keeps original order as tie-breaker
	// and skips atomics-first partition to reduce churn.
	PolicyStable Policy = "stable"
)

// ParsePolicy validates a policy name (empty → atomics).
func ParsePolicy(name string) (Policy, error) {
	switch Policy(name) {
	case "", PolicyAtomics:
		return PolicyAtomics, nil
	case PolicyDensity, PolicyStable:
		return Policy(name), nil
	default:
		return "", fmt.Errorf("unknown policy %q (atomics, density, stable)", name)
	}
}
