// Copyright 2019 DxChain, All rights reserved.
// Use of this source code is governed by an Apache
// License 2.0 that can be found in the LICENSE file

package storagehostmanager

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/DxChainNetwork/godx/common"
	"github.com/DxChainNetwork/godx/p2p/enode"
	"github.com/DxChainNetwork/godx/storage"
	"github.com/DxChainNetwork/godx/storage/storageclient/storagehosttree"
	"github.com/Pallinder/go-randomdata"
)

// StorageHostRank will be used to show the rankings of the storage host
// learnt by the storage client
type StorageHostRank struct {
	storagehosttree.EvaluationDetail
	EnodeID string
}

// hostInfoGenerator will randomly generate storage host information
func hostInfoGenerator() storage.HostInfo {
	ip := randomdata.IpV4Address()
	id := enodeIDGenerator()
	return storage.HostInfo{
		HostExtConfig: storage.HostExtConfig{
			AcceptingContracts:     true,
			Deposit:                common.NewBigInt(100),
			ContractPrice:          common.RandomBigInt(),
			DownloadBandwidthPrice: common.RandomBigInt(),
			StoragePrice:           common.RandomBigInt(),
			UploadBandwidthPrice:   common.RandomBigInt(),
			SectorAccessPrice:      common.RandomBigInt(),
			RemainingStorage:       100,
		},
		IP:       ip,
		EnodeID:  id,
		EnodeURL: fmt.Sprintf("enode://%s:%s:3030", id.String(), ip),
	}
}

// activeHostInfoGenerator will randomly generate active storage host information
func activeHostInfoGenerator() storage.HostInfo {
	ip := randomdata.IpV4Address()
	id := enodeIDGenerator()
	return storage.HostInfo{
		HostExtConfig: storage.HostExtConfig{
			AcceptingContracts:     true,
			Deposit:                common.RandomBigInt(),
			ContractPrice:          common.RandomBigInt(),
			DownloadBandwidthPrice: common.RandomBigInt(),
			StoragePrice:           common.RandomBigInt(),
			UploadBandwidthPrice:   common.RandomBigInt(),
			SectorAccessPrice:      common.RandomBigInt(),
			RemainingStorage:       100,
		},
		IP:       ip,
		EnodeID:  id,
		EnodeURL: fmt.Sprintf("enode://%s:%s:3030", id.String(), ip),
		ScanRecords: storage.HostPoolScans{
			storage.HostPoolScan{
				Timestamp: time.Now(),
				Success:   true,
			},
		},
	}
}

// enodeIDGenerator will randomly generate enode ID
func enodeIDGenerator() (id enode.ID) {
	rand.Read(id[:])
	return
}