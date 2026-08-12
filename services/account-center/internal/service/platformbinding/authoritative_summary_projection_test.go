package platformbinding

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrimaryOperationSummaryRequiresExactUniquePrimary(t *testing.T) {
	base := RuntimeSummary{
		Generation: 4, ProfileSnapshotComplete: true, ProfileRevision: 8, ProfileObservedRevision: 8,
	}
	tests := []struct {
		name     string
		profiles []map[string]any
		valid    bool
	}{
		{
			name: "requested profile is the only primary",
			profiles: []map[string]any{
				{"profile_ref": "profile-one", "is_default": false},
				{"profile_ref": "profile-two", "is_default": true},
			},
			valid: true,
		},
		{
			name:     "different profile is primary",
			profiles: []map[string]any{{"profile_ref": "profile-one", "is_default": true}},
		},
		{
			name: "multiple profiles are primary",
			profiles: []map[string]any{
				{"profile_ref": "profile-two", "is_default": true},
				{"profile_ref": "profile-two", "is_default": true},
			},
		},
		{name: "no profile is primary", profiles: []map[string]any{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			summary := base
			summary.Profiles = test.profiles
			require.Equal(t, test.valid, validOperationSummary("OPERATION_KIND_SET_PRIMARY_PROFILE", 4, 7, "profile-two", &summary))
		})
	}
}
