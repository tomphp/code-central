package factories_test

import (
	"github.com/tomoram/code-central/core/coretest"
)

type fixture = coretest.Fixture

var (
	ctx         = coretest.Ctx
	human       = coretest.Human
	t0          = coretest.T0
	newFixture  = coretest.New
	twoServices = coretest.TwoServices
	wantCode    = coretest.WantCode
	noNotice    = coretest.NoNotice
)
