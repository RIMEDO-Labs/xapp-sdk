package main

import (
	"github.com/RIMEDO-Labs/xapp-sdk/pkg/manager"
	"github.com/onosproject/onos-lib-go/pkg/logging"
)

var log = logging.GetLogger("main")

func main() {
	ready := make(chan bool)

	log.Info("Starting xapp-sdk")

	cfg := manager.Config{
		AppID:       "xapp-sdk",
		E2tAddress:  "onos-e2t",
		E2tPort:     5150,
		TopoAddress: "onos-topo",
		TopoPort:    5150,
		SMName:      "oran-e2sm-mho",
		SMVersion:   "v1",
	}

	mgr := manager.NewManager(cfg)
	mgr.Run()
	<-ready
}
