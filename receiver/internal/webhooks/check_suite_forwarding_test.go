package webhooks

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsAllowedAuthorAssociation(t *testing.T) {
	testCases := []struct {
		name                string
		allowedAssociations []string
		association         string
		expectedResult      bool
	}{
		{
			name:                "not allowed",
			allowedAssociations: []string{"OWNER"},
			association:         "COLLABORATOR",
			expectedResult:      false,
		},
		{
			name:                "allowed",
			allowedAssociations: []string{"OWNER", "COLLABORATOR"},
			association:         "COLLABORATOR",
			expectedResult:      true,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			s := &service{
				config: ServiceConfig{
					CheckSuiteAllowedAuthorAssociations: testCase.allowedAssociations,
				},
			}
			require.Equal(
				t,
				testCase.expectedResult,
				s.isAllowedAuthorAssociation(testCase.association),
			)
		})
	}
}
