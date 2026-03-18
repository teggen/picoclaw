package status

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCountAny(t *testing.T) {
	assert.Equal(t, 3, countAny([]any{"a", "b", "c"}))
	assert.Equal(t, 5, countAny(float64(5)))
	assert.Equal(t, 0, countAny("not a number"))
	assert.Equal(t, 0, countAny(nil))
}

func TestFormatSkills(t *testing.T) {
	assert.Equal(t, "3/5 available", formatSkills(map[string]any{
		"installed": float64(3),
		"total":     float64(5),
	}))
	assert.Equal(t, "3 installed", formatSkills(map[string]any{
		"installed": float64(3),
	}))
	assert.Equal(t, "custom", formatSkills("custom"))
	assert.Equal(t, "n/a", formatSkills(nil))
}

func TestJoinStrings(t *testing.T) {
	assert.Equal(t, "a, b, c", joinStrings([]string{"a", "b", "c"}, ", "))
	assert.Equal(t, "single", joinStrings([]string{"single"}, ", "))
	assert.Equal(t, "", joinStrings([]string{}, ", "))
}
