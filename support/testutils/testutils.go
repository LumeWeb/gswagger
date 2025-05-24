package testutils

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func AssertJSONMatchesFile(t *testing.T, actual []byte, filename string) {
	t.Helper()
	
	expected, err := os.ReadFile(filename)
	require.NoError(t, err)
	
	var expectedJSON, actualJSON interface{}
	
	err = json.Unmarshal(expected, &expectedJSON)
	require.NoError(t, err)
	
	err = json.Unmarshal(actual, &actualJSON)
	require.NoError(t, err)
	
	expectedFormatted, err := json.MarshalIndent(expectedJSON, "", "  ")
	require.NoError(t, err)
	
	actualFormatted, err := json.MarshalIndent(actualJSON, "", "  ")
	require.NoError(t, err)
	
	require.Equal(t, string(expectedFormatted), string(actualFormatted))
}
