package main

import (
	"github.com/RIMEDO-Labs/xapp-sdk/pkg/manager"
	"github.com/onosproject/onos-lib-go/pkg/logging"
)

var log = logging.GetLogger("Main source file [cmd/xpp-sdk.go]")

func main() {

	ready := make(chan bool)

	log.Info("Starting RIMEDO Labs xAPP SDK...")

	manager := manager.NewManager()
	manager.Run()

	<-ready

}
