package test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tsiemens/acb/util"
)

func TestMathMin(t *testing.T) {
	require.Equal(t, util.MinValue(50, 40), uint32(40))
	require.Equal(t, util.MinValue(40, 50, 60), uint32(40))
	require.Equal(t, util.MinValue(60, 50, 40), uint32(40))
}
