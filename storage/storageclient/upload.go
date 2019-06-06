// Copyright 2019 DxChain, All rights reserved.
// Use of this source code is governed by an Apache
// License 2.0 that can be found in the LICENSE file.

package storageclient

import (
	"fmt"
	"github.com/DxChainNetwork/godx/crypto"
	"github.com/DxChainNetwork/godx/storage"
	"github.com/DxChainNetwork/godx/storage/storageclient/erasurecode"
	"github.com/DxChainNetwork/godx/storage/storageclient/filesystem/dxdir"
	"github.com/DxChainNetwork/godx/storage/storageclient/filesystem/dxfile"
	"os"
)

// Upload instructs the renter to start tracking a file. The renter will
// automatically upload and repair tracked files using a background loop.
func (sc *StorageClient) Upload(up FileUploadParams) error {
	if err := sc.tm.Add(); err != nil {
		return err
	}
	defer sc.tm.Done()

	// Check whether file is a directory
	sourceInfo, err := os.Stat(up.Source)
	if err != nil {
		return fmt.Errorf("unable to stat input file, error: %v", err)
	}
	if sourceInfo.IsDir() {
		return dxdir.ErrUploadDirectory
	}

	file, err := os.Open(up.Source)
	if err != nil {
		return fmt.Errorf("unable to open the source file, error: %v", err)
	}
	file.Close()

	// Delete existing file if Override mode
	if up.Mode == Override {
		if err := sc.DeleteFile(up.DxPath); err != nil && err != dxdir.ErrUnknownPath {
			return fmt.Errorf("cannot to delete existing file, error: %v", err)
		}
	}

	// Setup ECTypeStandard's ErasureCode with default params
	if up.ErasureCode == nil {
		up.ErasureCode, _ = erasurecode.New(erasurecode.ECTypeStandard, DefaultMinSectors, DefaultNumSectors)
	}

	// TODO sc.contractManager.Contracts()
	numContracts := uint32(100) // len(sc.contractManager.Contracts())
	requiredContracts := (up.ErasureCode.NumSectors() + up.ErasureCode.MinSectors()) / 2
	if numContracts < requiredContracts {
		return fmt.Errorf("not enough contracts to upload file: got %v, needed %v", numContracts, (up.ErasureCode.NumSectors()+up.ErasureCode.MinSectors())/2)
	}

	dirDxPath := up.DxPath

	// Try to create the directory. If ErrPathOverload is returned it already exists
	dxDirEntry, err := sc.staticDirSet.NewDxDir(dirDxPath)
	if err != dxdir.ErrPathOverload && err != nil {
		return fmt.Errorf("unable to create dx directory for new file, error: %v", err)
	} else if err == nil {
		dxDirEntry.Close()
	}

	cipherKey, err := crypto.GenerateCipherKey(crypto.GCMCipherCode)
	if err != nil {
		return fmt.Errorf("generate cipher key error: %v", err)
	}
	// Create the DxFile and add to client
	entry, err := sc.staticFileSet.NewDxFile(up.DxPath, storage.SysPath(up.Source), up.Mode == Override, up.ErasureCode, cipherKey, uint64(sourceInfo.Size()), sourceInfo.Mode())
	if err != nil {
		return fmt.Errorf("could not create a new dx file, error: %v", err)
	}
	defer entry.Close()

	if sourceInfo.Size() == 0 {
		return nil
	}

	// Bubble the health of the DxFile directory to ensure the health is updated with the new file
	go sc.fileSystem.InitAndUpdateDirMetadata(dirDxPath)

	nilHostHealthInfoTable := make(storage.HostHealthInfoTable)

	// Send the upload to the repair loop
	hosts := sc.refreshHostsAndWorkers()
	sc.createAndPushSegments([]*dxfile.FileSetEntryWithID{entry}, hosts, targetUnstuckSegments, nilHostHealthInfoTable)
	select {
	case sc.uploadHeap.newUploads <- struct{}{}:
	default:
	}
	return nil
}
