package gateway

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
)

// TestUnitGatewaySenderSuite go test 执行入口
func TestUnitGatewaySenderSuite(t *testing.T) {
	suite.Run(t, new(gatewaySenderSuite))
}

type gatewaySenderSuite struct {
	suite.Suite
	fakeErr error
}

// SetupTest 执行用例前初始化
func (g *gatewaySenderSuite) SetupTest() {
	g.fakeErr = errors.New("fake error")
}
