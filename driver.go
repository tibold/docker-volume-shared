// +build linux
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	log "github.com/Sirupsen/logrus"

	"github.com/davecgh/go-spew/spew"
	"github.com/docker/go-plugins-helpers/volume"
)

// A single volume instance
type beegfsMount struct {
	name string
	path string
	root string
	keep bool
}

type beegfsDriver struct {
	mounts map[string]*beegfsMount
	m      *sync.Mutex
}

func newBeeGFSDriver(root string) beegfsDriver {
	d := beegfsDriver{
		mounts: make(map[string]*beegfsMount),
		m:      &sync.Mutex{},
	}

	return d
}

func (b beegfsDriver) Create(r *volume.CreateRequest) error {
	var volumeRoot string
	var volumeKeep bool

	log.Infof("Create: %s, %v", r.Name, r.Options)

	b.m.Lock()
	defer b.m.Unlock()

	// Handle options (unrecognized options are silently ignored):
	// root: directory to create new volumes (this should correspond with
	//       beegfs-mounts.conf).
	if optsRoot, ok := r.Options["root"]; ok {
		volumeRoot = optsRoot
	} else {
		// Assume the default root
		volumeRoot = *root
	}

	if optsKeep, ok := r.Options["keep"]; ok {
		if parsedKeep, err := strconv.ParseBool(optsKeep); err == nil {
			volumeKeep = parsedKeep
		}
	}

	dest := filepath.Join(volumeRoot, r.Name)
	if !isbeegfs(dest) {
		emsg := fmt.Sprintf("Cannot create volume %s as it's not on a BeeGFS filesystem", dest)
		log.Error(emsg)
		return errors.New(emsg)
	}

	fmt.Printf("mounts: %d", len(b.mounts))
	if _, ok := b.mounts[r.Name]; ok {
		imsg := fmt.Sprintf("Cannot create volume %s, it already exists", dest)
		log.Info(imsg)
		return nil
	}

	volumePath := filepath.Join(volumeRoot, r.Name)

	if err := createDest(dest); err != nil {
		return err
	}

	mount := &beegfsMount{
		name: r.Name,
		path: volumePath,
		root: volumeRoot,
		keep: volumeKeep,
	}
	b.mounts[r.Name] = mount
	saveMountInfo(mount)

	if *verbose {
		spew.Dump(b.mounts)
	}

	return nil
}

func (b beegfsDriver) Remove(r *volume.RemoveRequest) error {
	log.Infof("Remove: %s", r.Name)

	b.m.Lock()
	defer b.m.Unlock()

	if mount, ok := b.mounts[r.Name]; ok {

		loadMountInfo(mount)

		if !mount.keep {
			os.RemoveAll(mount.path)
		}

		delete(b.mounts, r.Name)
	}

	return nil
}

func (b beegfsDriver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	log.Debugf("Path: %s", r.Name)

	if _, ok := b.mounts[r.Name]; ok {
		return &volume.PathResponse{Mountpoint: b.mounts[r.Name].path}, nil
	}

	return nil, nil
}

func (b beegfsDriver) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	log.Infof("Mount: %s", r.Name)
	dest := filepath.Join(b.mounts[r.Name].root, r.Name)

	if !isbeegfs(dest) {
		emsg := fmt.Sprintf("Cannot mount volume %s as it's not on a BeeGFS filesystem", dest)
		log.Error(emsg)
		return nil, errors.New(emsg)
	}

	if _, ok := b.mounts[r.Name]; ok {
		return &volume.MountResponse{Mountpoint: b.mounts[r.Name].path}, nil
	}

	return nil, nil
}

func (b beegfsDriver) Unmount(r *volume.UnmountRequest) error {
	log.Infof("Unmount: %s", r.Name)
	return nil
}

func (b beegfsDriver) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	log.Infof("Get: %s", r.Name)

	discoverVolumes(&b)

	if v, ok := b.mounts[r.Name]; ok {
		return &volume.GetResponse{
			Volume: &volume.Volume{
				Name:       v.name,
				Mountpoint: v.path,
			},
		}, nil
	}

	return nil, fmt.Errorf("volume %s unknown", r.Name)
}

func (b beegfsDriver) List() (*volume.ListResponse, error) {
	log.Infof("List")

	discoverVolumes(&b)

	volumes := []*volume.Volume{}

	for v := range b.mounts {
		if isbeegfs(b.mounts[v].path) {
			volumes = append(volumes, &volume.Volume{Name: b.mounts[v].name, Mountpoint: b.mounts[v].path})
		} else {
			// Volume must have been remove by others.
			log.Infof("Volume %s was removed by other nodes.", v)
			delete(b.mounts, v)
		}
	}

	return &volume.ListResponse{Volumes: volumes}, nil
}

func (b beegfsDriver) Capabilities() *volume.CapabilitiesResponse {
	return &volume.CapabilitiesResponse{
		Capabilities: volume.Capability{
			Scope: "global",
		},
	}
}

func discoverVolumes(driver *beegfsDriver) {

	log.Infof("Volume discovery.")
	if files, err := ioutil.ReadDir(*root); err != nil {
		for _, file := range files {
			log.Infof("Testing %s if it is a volume.", file.Name())
			if file.IsDir() {
				name := file.Name()
				if _, ok := driver.mounts[name]; !ok {

					log.Infof("Discovered volume %s.", name)

					driver.mounts[name] = &beegfsMount{
						name: name,
						path: filepath.Join(*root, name),
						root: *root,
						keep: false,
					}
				}
			}
		}
	}
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

	log.Debugf("Type for %s: %d", volumepath, stat.Type)

	// BEEGFS_MAGIC 0x19830326
	return stat.Type == 428016422
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

func saveMountInfo(mount *beegfsMount) {
	metaFile := filepath.Join(mount.path, "meta.json")

	if content, err := json.MarshalIndent(mount, "", ""); err == nil {
		_ = ioutil.WriteFile(metaFile, content, 0600)
	}
}

func loadMountInfo(mount *beegfsMount) {

	metaFile := filepath.Join(mount.path, "meta.json")

	if content, err := ioutil.ReadFile(metaFile); err == nil {
		_ = json.Unmarshal([]byte(content), &mount)
	}
}
