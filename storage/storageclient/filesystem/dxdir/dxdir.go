// Copyright 2019 DxChain, All rights reserved.
// Use of this source code is governed by an Apache
// License 2.0 that can be found in the LICENSE file.

package dxdir

import (
	"os"
	"sync"
	"time"

	"github.com/DxChainNetwork/godx/common/math"
	"github.com/DxChainNetwork/godx/common/writeaheadlog"
	"github.com/DxChainNetwork/godx/storage"
)

const (
	DirFileName = ".dxdir"

	DefaultHealth = uint32(200)
)

type (
	// DxDir is the data structure for the directory for the meta info for a directory.
	DxDir struct {
		// metadata
		metadata *Metadata

		// utilities
		deleted bool
		lock    sync.RWMutex
		wal     *writeaheadlog.Wal

		// dirFilePath is the full path of the DxDir file
		dirFilePath storage.SysPath
	}

	// Metadata is the necessary metadata to be saved in DxDir
	Metadata struct {
		// Total number of files in directory and its subdirectories
		NumFiles uint64

		// Total size of the directory and its subdirectories
		TotalSize uint64

		// Health is the min Health all files and subdirectories
		Health uint32

		// StuckHealth is the min StuckHealth for all files and subdirectories
		StuckHealth uint32

		// MinRedundancy is the minimum redundancy
		MinRedundancy uint32

		// TimeLastHealthCheck is the last health check time
		TimeLastHealthCheck uint64

		// TimeModify is the last content modification time
		TimeModify uint64

		// NumStuckSegments is the total number of segments that is stuck
		NumStuckSegments uint32

		// DxPath is the DxPath which is the path related to the root directory
		DxPath storage.DxPath

		// RootPath is the root path of the file directory
		RootPath storage.SysPath
	}
)

//New create a DxDir with representing the dirPath metadata.
//Note that the only access method should be from dirSet
func New(dxPath storage.DxPath, rootPath storage.SysPath, wal *writeaheadlog.Wal) (*DxDir, error) {
	filePath := rootPath.Join(dxPath, DirFileName)
	_, err := os.Stat(string(filePath))
	if err == nil {
		return nil, os.ErrExist
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	if err = os.MkdirAll(string(rootPath.Join(dxPath)), 0700); err != nil {
		return nil, err
	}
	metadata := &Metadata{
		Health:        DefaultHealth,
		StuckHealth:   DefaultHealth,
		MinRedundancy: math.MaxUint32,
		TimeModify:    uint64(time.Now().Unix()),
		DxPath:        dxPath,
		RootPath:      rootPath,
	}
	d := &DxDir{
		metadata:    metadata,
		deleted:     false,
		wal:         wal,
		dirFilePath: filePath,
	}
	err = d.save()
	if err != nil {
		return nil, err
	}
	return d, nil
}

// Delete delete the dxfile
func (d *DxDir) Delete() error {
	d.lock.Lock()
	defer d.lock.Unlock()

	return d.delete()
}

// Deleted return the delete status
func (d *DxDir) Deleted() bool {
	d.lock.RLock()
	defer d.lock.RUnlock()

	return d.deleted
}

// Metadata return the copy of the Metadata
func (d *DxDir) Metadata() Metadata {
	d.lock.RLock()
	defer d.lock.RUnlock()

	return *d.metadata
}

// DxPath return the DxPath of the Dxdir
func (d *DxDir) DxPath() storage.DxPath {
	d.lock.RLock()
	defer d.lock.RUnlock()

	return d.metadata.DxPath
}

// filePath return the actual dxdir file path of a dxdir.
func (d *DxDir) FilePath() string {
	return string(d.dirFilePath)
}

// UpdateMetadata update the metadata with the given metadata.
// Not the DxPath field is not updated
func (d *DxDir) UpdateMetadata(metadata Metadata) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	// Update the fields
	d.metadata.NumFiles = metadata.NumFiles
	d.metadata.TotalSize = metadata.TotalSize
	d.metadata.Health = metadata.Health
	d.metadata.StuckHealth = metadata.StuckHealth
	d.metadata.MinRedundancy = metadata.MinRedundancy
	d.metadata.TimeLastHealthCheck = metadata.TimeLastHealthCheck
	d.metadata.TimeModify = uint64(time.Now().Unix())
	d.metadata.NumStuckSegments = metadata.NumStuckSegments

	// DxPath and RootPath field should never be updated
	return d.save()
}
