package main

import (
	"flag"
	"fmt"
	"os"

	log "github.com/Sirupsen/logrus"

	"github.com/docker/go-plugins-helpers/volume"
)

var (
	// This is the path in beegfs-mounts.conf
	root     = flag.String("root", "", "Base directory where volumes are created in the cluster")
	debug    = flag.Bool("debug", true, "Enable verbose logging")
	hostname = flag.String("hostname", "", "The hostname used in locking operations")
)

func main() {
	flag.Parse()

	if *debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	if *hostname == "" {
		*hostname, _ = os.Hostname()
	}

	log.Debugf("Starting with hostname=%s; root=%s", *hostname, *root)

	// userID, _ := user.Lookup("root")
	// groupID, _ := strconv.Atoi(userID.Gid)

	driver := newBeeGFSDriver(*root)
	handler := volume.NewHandler(driver)
	fmt.Println(handler.ServeUnix("shared", 0))
}
