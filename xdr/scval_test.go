package xdr

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/lantah/go/gxdr"
	"github.com/lantah/go/randxdr"
)

func TestScValEqualsCoverage(t *testing.T) {
	gen := randxdr.NewGenerator()
	for i := 0; i < 30000; i++ {
		scVal := ScVal{}

		shape := &gxdr.SCVal{}
		gen.Next(
			shape,
			[]randxdr.Preset{},
		)
		assert.NoError(t, gxdr.Convert(shape, &scVal))

		clonedScVal := ScVal{}
		assert.NoError(t, gxdr.Convert(shape, &clonedScVal))
		assert.True(t, scVal.Equals(clonedScVal), "scVal: %#v, clonedScVal: %#v", scVal, clonedScVal)
	}
}
