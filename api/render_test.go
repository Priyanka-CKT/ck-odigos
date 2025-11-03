package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCRD(t *testing.T) {
	crds, err := GetCRDs([]string{"codekarma.tech_instrumentationconfigs.yaml"})
	assert.NoError(t, err)
	assert.NotEmpty(t, crds)
	for _, crd := range crds {
		assert.NotEmpty(t, "instrumentationconfigs.codekarma.tech", crd.Name)
	}
}
