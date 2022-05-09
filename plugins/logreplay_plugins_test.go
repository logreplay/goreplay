package plugins

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

// TestUnitLogreplayPlugin logreplay unit test execute
func TestUnitLogreplayPlugin(t *testing.T) {
	suite.Run(t, new(logreplayPluginSuite))
}

type logreplayPluginSuite struct {
	suite.Suite
}
