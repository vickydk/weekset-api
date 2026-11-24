package exercise

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogHasTwelveMovements(t *testing.T) {
	assert.Len(t, Catalog, 12)
	seen := map[string]bool{}
	for _, movement := range Catalog {
		assert.NotEmpty(t, movement.Slug)
		assert.NotEmpty(t, movement.Name)
		assert.False(t, seen[movement.Slug], movement.Slug)
		seen[movement.Slug] = true
	}
}

func TestMigrationSeedsTheCatalog(t *testing.T) {
	body, err := os.ReadFile("../../../migrations/000001_schema.up.sql")
	require.NoError(t, err)
	sql := string(body)
	for _, movement := range Catalog {
		assert.Contains(t, sql, "'"+movement.Slug+"'")
		assert.Contains(t, sql, "'"+movement.Name+"'")
	}
}
