package manager

import (
	"github.com/onosproject/onos-lib-go/pkg/logging"
)

var log = logging.GetLogger("manager")

func NewManager() {
	// for testing only
	log.Info("Created New Manager")
}
