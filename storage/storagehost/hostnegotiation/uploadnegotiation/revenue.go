// Copyright 2019 DxChain, All rights reserved.
// Use of this source code is governed by an Apache
// License 2.0 that can be found in the LICENSE file

package uploadnegotiation

import (
	"github.com/DxChainNetwork/godx/common"
	"github.com/DxChainNetwork/godx/storage"
	"github.com/DxChainNetwork/godx/storage/storagehost"
	"github.com/DxChainNetwork/godx/storage/storagehost/hostnegotiation"
)

// calcHostRevenue calculates the host revenue for upload data for the storage client. The revenue includes
// the bandwidth revenue, storage revenue, and baseRPCPrice
func calcHostRevenue(session *hostnegotiation.UploadSession, sr storagehost.StorageResponsibility, blockHeight uint64, hostConfig storage.HostIntConfig) common.BigInt {
	calcStorageRevenueAndNewDeposit(session, sr, blockHeight, hostConfig.StoragePrice, hostConfig.Deposit)
	return session.StorageRevenue.Add(session.BandwidthRevenue).Add(hostConfig.BaseRPCPrice)
}

// calcStorageRevenueAndNewDeposit calculates both storage revenue and the new deposit
func calcStorageRevenueAndNewDeposit(session *hostnegotiation.UploadSession, sr storagehost.StorageResponsibility, blockHeight uint64, storagePrice, deposit common.BigInt) {
	if len(session.SectorRoots) > len(sr.SectorRoots) {
		dataAdded := storage.SectorSize * uint64(len(session.SectorRoots)-len(sr.SectorRoots))
		blocksRemaining := sr.ProofDeadline() - blockHeight
		dataBlocks := common.NewBigIntUint64(blocksRemaining).Mult(common.NewBigIntUint64(dataAdded))
		session.StorageRevenue = dataBlocks.Mult(storagePrice)
		session.NewDeposit = dataBlocks.Mult(deposit)
	}
}

// calcBandwidthRevenueForProof calculates the bandwidth revenue for storage merkle proof
// the revenue also includes the bandwidth revenue for upload the data calculated previously
func calcBandwidthRevenueWithProof(session *hostnegotiation.UploadSession, subTreeHashesLen, leafHashesLen int, downloadBandwidthPrice common.BigInt) common.BigInt {
	// calculate the merkle proof size
	merkleProofSize := storage.HashSize * (subTreeHashesLen + leafHashesLen + 1)
	session.BandwidthRevenue = session.BandwidthRevenue.Add(downloadBandwidthPrice.Mult(common.NewBigInt(int64(merkleProofSize))))
	return session.BandwidthRevenue
}