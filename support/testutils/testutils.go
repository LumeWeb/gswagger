package testutils

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// AssertJSONMatchesFile reads a JSON file and compares its content with the provided byte slice.
// It unmarshals both into generic Go types (map[string]interface{}) and uses require.Equal
// for an order-insensitive comparison.
func AssertJSONMatchesFile(t *testing.T, actual []byte, filename string) {
	t.Helper()

	expected, err := os.ReadFile(filename)
	require.NoError(t, err)

	var expectedJSON, actualJSON interface{}

	err = json.Unmarshal(expected, &expectedJSON)
	require.NoError(t, err)

	err = json.Unmarshal(actual, &actualJSON)
	require.NoError(t, err)

	// Use require.Equal for deep comparison of the unmarshaled structures
	require.Equal(t, expectedJSON, actualJSON, "JSON mismatch with file %s", filename)
}
