package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"os"
	"path"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/calavera/dkvolume"
)

// unused for the moment -- ought to be used by 'docker volume create [...] --opt beegfsbaseOpt=bla
const (
	beegfsbaseOpt = "/mnt/beegfs"
)

type beegfsDriver struct {
	root string
	m    *sync.Mutex
}

func newBeeGFSDriver(root string) beegfsDriver {
	d := beegfsDriver{
		root: root,
	}

	return d
}

func (b beegfsDriver) Create(r dkvolume.Request) dkvolume.Response {
	log.Debugf("Create: %s, %v", r.Name, r.Options)
	dest := volumeDir(b, r)

	if !isbeegfs(dest) {
		emsg := fmt.Sprintf("Cannot create volume %s as it's not on a BeeGFS filesystem", dest)
		log.Error(emsg)
		return dkvolume.Response{Err: emsg}
	}

	if err := createDest(dest); err != nil {
		return dkvolume.Response{Err: err.Error()}
	}

	return dkvolume.Response{}
}

func (b beegfsDriver) Remove(r dkvolume.Request) dkvolume.Response {
	log.Debugf("Remove: %s", r.Name)
	return dkvolume.Response{}
}

func (b beegfsDriver) Path(r dkvolume.Request) dkvolume.Response {
	log.Debugf("Path: %s", r.Name)
	return dkvolume.Response{Mountpoint: volumeDir(b, r)}
}

func (b beegfsDriver) Mount(r dkvolume.Request) dkvolume.Response {
	log.Debugf("Mount: %s", r.Name)
	dest := volumeDir(b, r)

	if !isbeegfs(dest) {
		emsg := fmt.Sprintf("Cannot mount volume %s as it's not on a BeeGFS filesystem", dest)
		log.Error(emsg)
		return dkvolume.Response{Err: emsg}
	}

	return dkvolume.Response{Mountpoint: dest}
}

func (b beegfsDriver) Unmount(r dkvolume.Request) dkvolume.Response {
	log.Debugf("Unmount: %s", r.Name)
	return dkvolume.Response{}
}

func volumeDir(b beegfsDriver, r dkvolume.Request) string {
	return filepath.Join(b.root, r.Name)
}

// Check if the parent directory (where the volume will be created)
// is of type 'beegfs' using the BEEGFS_MAGIC value.
func isbeegfs(volumepath string) bool {
	log.Debugf("isbeegfs() for %s", volumepath)
	stat := syscall.Statfs_t{}
	err := syscall.Statfs(path.Dir(volumepath), &stat)
	if err != nil {
		log.Errorf("Could not determine filesystem type for %s: %s", volumepath, err)
		return false
	}

        // BEEGFS_MAGIC 0x19830326
	return stat.Type == int64(428016422)
}

func createDest(dest string) error {
	fstat, err := os.Lstat(dest)

	if os.IsNotExist(err) {
		if err := os.MkdirAll(dest, 0755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if fstat != nil && !fstat.IsDir() {
		return fmt.Errorf("%v already exist and it's not a directory", dest)
	}

	return nil
}