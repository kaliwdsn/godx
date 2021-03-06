// Copyright 2019 DxChain, All rights reserved.
// Use of this source code is governed by an Apache
// License 2.0 that can be found in the LICENSE file.

package dxfile

import (
	"fmt"
	"os"
	"time"

	"github.com/DxChainNetwork/godx/crypto"
	"github.com/DxChainNetwork/godx/log"
	"github.com/DxChainNetwork/godx/storage"
	"github.com/DxChainNetwork/godx/storage/storageclient/erasurecode"
)

type (
	// Metadata is the Metadata of a user uploaded file.
	Metadata struct {
		ID FileID

		// storage related
		HostTableOffset uint64
		SegmentOffset   uint64

		// size related
		FileSize   uint64
		SectorSize uint64 // ShardSize is the size for one shard, which is by default 4MiB

		// path related
		LocalPath storage.SysPath // Local path is the on-disk location for uploaded files
		DxPath    storage.DxPath  // DxPath is the user specified dxpath

		// Encryption
		CipherKeyCode uint8  // cipher key code defined in cipher package
		CipherKey     []byte // Key used to encrypt pieces

		// Time fields. most of unix timestamp
		TimeModify uint64 // time of last content modification
		TimeUpdate uint64 // time of last Metadata update
		TimeAccess uint64 // time of last access
		TimeCreate uint64 // time of file creation

		// Repair loop fields
		Health              uint32 // Worst health of the file's unstuck segment
		StuckHealth         uint32 // Worst health of the file's Stuck segment
		TimeLastHealthCheck uint64 // Time of last health check happenning
		NumStuckSegments    uint32 // Number of Stuck segments
		TimeRecentRepair    uint64 // Timestamp of last segment repair
		LastRedundancy      uint32 // File redundancy from last check

		// File related
		FileMode os.FileMode // unix file mode

		// Erasure code field
		ErasureCodeType uint8  // the code for the specific erasure code
		MinSectors      uint32 // params for erasure coding. The number of slice raw Data split into.
		NumSectors      uint32 // params for erasure coding. The number of total Sectors
		ECExtra         []byte // extra parameters for erasure code

		// Version control for fork
		Version string
	}

	// UpdateMetaData is the Metadata to be updated
	UpdateMetaData struct {
		Health           uint32
		StuckHealth      uint32
		LastHealthCheck  time.Time
		NumStuckSegments uint64
		RecentRepairTime time.Time
		Redundancy       uint32
		Size             uint64
		TimeModify       time.Time
	}

	// CachedHealthMetadata is a helper struct that contains the dxfile health
	// metadata fields that are cached
	CachedHealthMetadata struct {
		Health      uint32
		Redundancy  uint32
		StuckHealth uint32
	}
)

// LocalPath return the local path of a file
func (df *DxFile) LocalPath() storage.SysPath {
	df.lock.RLock()
	defer df.lock.RUnlock()
	return df.metadata.LocalPath
}

// SetLocalPath change the value of local path and save to disk
func (df *DxFile) SetLocalPath(path storage.SysPath) error {
	df.lock.RLock()
	defer df.lock.RUnlock()

	df.metadata.LocalPath = path
	return df.saveMetadata()
}

// DxPath return dxfile.metadata.DxPath
func (df *DxFile) DxPath() storage.DxPath {
	df.lock.RLock()
	defer df.lock.RUnlock()
	return df.metadata.DxPath
}

// FilePath return the actual file path of the dxfile
func (df *DxFile) FilePath() string {
	df.lock.RLock()
	defer df.lock.RUnlock()
	return string(df.filePath)
}

// FileSize return the file size of the dxfile
func (df *DxFile) FileSize() uint64 {
	df.lock.RLock()
	defer df.lock.RUnlock()
	return df.metadata.FileSize
}

// TimeModify return the TimeModify of a DxFile
func (df *DxFile) TimeModify() time.Time {
	df.lock.RLock()
	defer df.lock.RUnlock()
	if int64(df.metadata.TimeModify) < 0 {
		log.Error("TimeModify uint64 overflow")
		return time.Time{}
	}
	return time.Unix(int64(df.metadata.TimeModify), 0)
}

// TimeAccess return the last access time of a DxFile
func (df *DxFile) TimeAccess() time.Time {
	df.lock.RLock()
	defer df.lock.RUnlock()
	if int64(df.metadata.TimeAccess) < 0 {
		log.Error("TimeAccess uint64 overflow")
		return time.Time{}
	}
	return time.Unix(int64(df.metadata.TimeAccess), 0)
}

// SetTimeAccess set df.metadata.TimeAccess
func (df *DxFile) SetTimeAccess(t time.Time) error {
	df.lock.RLock()
	defer df.lock.RUnlock()
	df.metadata.TimeAccess = uint64(t.Unix())
	return df.saveMetadata()
}

// TimeUpdate return the last update time of a DxFile
func (df *DxFile) TimeUpdate() time.Time {
	df.lock.RLock()
	defer df.lock.RUnlock()
	if int64(df.metadata.TimeUpdate) < 0 {
		log.Error("TimeUpdate uint64 overflow")
		return time.Time{}
	}
	return time.Unix(int64(df.metadata.TimeUpdate), 0)
}

// TimeCreate returns the TimeCreate of a DxFile
func (df *DxFile) TimeCreate() time.Time {
	df.lock.RLock()
	defer df.lock.RUnlock()
	if int64(df.metadata.TimeCreate) < 0 {
		log.Error("TimeCreate uint64 overflow")
		return time.Time{}
	}
	return time.Unix(int64(df.metadata.TimeCreate), 0)
}

// TimeLastHealthCheck return TimeHealthCheck
func (df *DxFile) TimeLastHealthCheck() time.Time {
	df.lock.RLock()
	defer df.lock.RUnlock()
	if int64(df.metadata.TimeRecentRepair) < 0 {
		log.Error("TimeRecentRepair uint64 overflow")
		return time.Time{}
	}
	return time.Unix(int64(df.metadata.TimeLastHealthCheck), 0)
}

// SetTimeLastHealthCheck set and save df.metadata.TimeLastHealthCheck
func (df *DxFile) SetTimeLastHealthCheck(t time.Time) error {
	df.lock.RLock()
	defer df.lock.RUnlock()
	df.metadata.TimeLastHealthCheck = uint64(t.Unix())
	return df.saveMetadata()
}

// LastTimeRecentRepair return df.metadata.LastTimeRecentRepair
func (df *DxFile) LastTimeRecentRepair() time.Time {
	df.lock.RLock()
	defer df.lock.RUnlock()
	if int64(df.metadata.TimeRecentRepair) < 0 {
		log.Error("TimeRecentRepair uint64 overflow")
		return time.Time{}
	}
	return time.Unix(int64(df.metadata.TimeRecentRepair), 0)
}

// SetTimeRecentRepair set and save df.metadata.TimeRecentRepair
func (df *DxFile) SetTimeRecentRepair(t time.Time) error {
	df.lock.Lock()
	defer df.lock.Unlock()

	df.metadata.TimeRecentRepair = uint64(t.Unix())
	return df.saveMetadata()
}

// SegmentSize return the size of a Segment for a DxFile.
func (df *DxFile) SegmentSize() uint64 {
	df.lock.RLock()
	defer df.lock.RUnlock()
	return df.metadata.segmentSize()
}

// CipherKey return the cipher key
func (df *DxFile) CipherKey() (crypto.CipherKey, error) {
	df.lock.RLock()
	defer df.lock.RUnlock()

	if df.cipherKey != nil {
		return df.cipherKey, nil
	}
	key, err := crypto.NewCipherKey(df.metadata.CipherKeyCode, df.metadata.CipherKey)
	if err != nil {
		// this should never happen
		log.Error("New Cipher Key return an error: %v", err)
		return nil, fmt.Errorf("new Cipher Key return an error: %v", err)
	}
	return key, nil
}

// ErasureCode return the erasure code
func (df *DxFile) ErasureCode() (erasurecode.ErasureCoder, error) {
	df.lock.RLock()
	defer df.lock.RUnlock()

	if df.erasureCode != nil {
		return df.erasureCode, nil
	}
	ec, err := erasurecode.New(df.metadata.ErasureCodeType, df.metadata.MinSectors, df.metadata.NumSectors,
		df.metadata.ECExtra)
	if err != nil {
		// this shall not happen
		log.Error("New erasure code return an error: %v", err)
		return ec, err
	}
	return ec, nil
}

// FileMode return the os file mode of a dxfile
func (df *DxFile) FileMode() os.FileMode {
	df.lock.RLock()
	defer df.lock.RUnlock()

	return df.metadata.FileMode
}

// SetFileMode change the value of df.metadata.FileMode and save it to file
func (df *DxFile) SetFileMode(mode os.FileMode) error {
	df.lock.Lock()
	defer df.lock.Unlock()

	df.metadata.FileMode = mode

	return df.saveMetadata()
}

// SectorSize return the Sector size of a dxfile
func (df *DxFile) SectorSize() uint64 {
	df.lock.RLock()
	defer df.lock.RUnlock()

	return df.metadata.SectorSize
}
