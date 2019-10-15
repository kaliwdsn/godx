// Copyright 2019 DxChain, All rights reserved.
// Use of this source code is governed by an Apache
// License 2.0 that can be found in the LICENSE file.

package storageclient

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DxChainNetwork/godx/common"
	"github.com/DxChainNetwork/godx/common/threadmanager"
	"github.com/DxChainNetwork/godx/core/types"
	"github.com/DxChainNetwork/godx/core/vm"
	"github.com/DxChainNetwork/godx/crypto/merkle"
	"github.com/DxChainNetwork/godx/internal/ethapi"
	"github.com/DxChainNetwork/godx/log"
	"github.com/DxChainNetwork/godx/p2p/enode"
	"github.com/DxChainNetwork/godx/rlp"
	"github.com/DxChainNetwork/godx/storage"
	"github.com/DxChainNetwork/godx/storage/storageclient/contractmanager"
	"github.com/DxChainNetwork/godx/storage/storageclient/filesystem"
	"github.com/DxChainNetwork/godx/storage/storageclient/memorymanager"
	"github.com/DxChainNetwork/godx/storage/storageclient/storagehostmanager"
)

// StorageClient contains fields that are used to perform StorageHost
// selection operation, file uploading, downloading operations, and etc.
type StorageClient struct {
	fileSystem filesystem.FileSystem

	// Memory Management
	memoryManager *memorymanager.MemoryManager

	storageHostManager *storagehostmanager.StorageHostManager
	contractManager    *contractmanager.ContractManager

	// Download management
	downloadHeapMu sync.Mutex
	downloadHeap   *downloadSegmentHeap
	newDownloads   chan struct{}

	// Upload management
	uploadHeap uploadHeap

	// List of workers that can be used for uploading and/or downloading.
	workerPool map[storage.ContractID]*worker

	// Directories and File related
	persist        persistence
	persistDir     string
	staticFilesDir string

	//storage client is used as the address to sign the storage contract and pays for the money
	PaymentAddress common.Address

	// Utilities
	log  log.Logger
	lock sync.Mutex
	tm   threadmanager.ThreadManager

	// information on network, block chain, and etc.
	info       storage.ParsedAPI
	ethBackend storage.EthBackend
	apiBackend ethapi.Backend
}

// New initializes StorageClient object
func New(persistDir string) (*StorageClient, error) {
	var err error

	sc := &StorageClient{
		persistDir:     persistDir,
		staticFilesDir: filepath.Join(persistDir, DxPathRoot),
		log:            log.New(),
		newDownloads:   make(chan struct{}, 1),
		downloadHeap:   new(downloadSegmentHeap),
		uploadHeap: uploadHeap{
			pendingSegments:     make(map[uploadSegmentID]struct{}),
			segmentComing:       make(chan struct{}, 1),
			stuckSegmentSuccess: make(chan storage.DxPath, 1),
		},
		workerPool: make(map[storage.ContractID]*worker),
	}

	sc.memoryManager = memorymanager.New(DefaultMaxMemory, sc.tm.StopChan())

	// initialize storageHostManager
	sc.storageHostManager = storagehostmanager.New(sc.persistDir)

	// initialize storage contract manager
	if sc.contractManager, err = contractmanager.New(sc.persistDir, sc.storageHostManager); err != nil {
		err = fmt.Errorf("error initializing contract manager: %s", err.Error())
		return nil, err
	}

	// initialize fileSystem
	sc.fileSystem = filesystem.New(persistDir, sc.contractManager)

	return sc, nil
}

// Start controls go routine checking and updating process
func (client *StorageClient) Start(b storage.EthBackend, apiBackend ethapi.Backend) (err error) {
	// get the eth backend
	client.ethBackend = b

	// getting all needed API functions
	if err = storage.FilterAPIs(b.APIs(), &client.info); err != nil {
		return
	}

	// start storageHostManager
	if err = client.storageHostManager.Start(client); err != nil {
		return
	}

	// start contractManager
	if err = client.contractManager.Start(client); err != nil {
		err = fmt.Errorf("error starting contract manager: %s", err.Error())
		return
	}

	// Load settings from persist file
	if err := client.loadPersist(); err != nil {
		return err
	}

	if err = client.fileSystem.Start(); err != nil {
		return err
	}

	// active the work pool to get a worker for a upload/download task.
	client.activateWorkerPool()

	// loop to download, upload, stuck and health check
	go client.downloadLoop()
	go client.uploadLoop()
	go client.stuckLoop()
	go client.uploadOrRepair()
	go client.healthCheckLoop()

	// kill workers on shutdown.
	client.tm.OnStop(func() error {
		client.lock.Lock()
		for _, worker := range client.workerPool {
			close(worker.killChan)
		}
		client.lock.Unlock()
		return nil
	})

	client.log.Info("Storage Client Started")

	return nil
}

// Close method will be used to send storage
func (client *StorageClient) Close() error {
	client.log.Info("Closing The Contract Manager")
	client.contractManager.Stop()

	var fullErr error

	// Closing the host manager
	client.log.Info("Closing the storage client host manager")
	err := client.storageHostManager.Close()
	fullErr = common.ErrCompose(fullErr, err)

	// Closing the file system
	client.log.Info("Closing the storage client file system")
	err = client.fileSystem.Close()
	fullErr = common.ErrCompose(fullErr, err)

	// Closing the thread manager
	client.log.Info("Closing The Storage Client Manager")
	err = client.tm.Stop()
	fullErr = common.ErrCompose(fullErr, err)
	return fullErr
}

// DeleteFile will delete from the file system file set. The file
// wil also be deleted from the disk
func (client *StorageClient) DeleteFile(path storage.DxPath) error {
	if err := client.tm.Add(); err != nil {
		return err
	}
	defer client.tm.Done()
	return client.fileSystem.DeleteDxFile(path)
}

// ContractDetail will return the detailed contract information
func (client *StorageClient) ContractDetail(contractID storage.ContractID) (detail storage.ContractMetaData, exists bool) {
	return client.contractManager.RetrieveActiveContract(contractID)
}

// ActiveContracts will retrieve all active contracts, reformat them, and return them back
func (client *StorageClient) ActiveContracts() (activeContracts []ActiveContractsAPIDisplay) {
	allActiveContracts := client.contractManager.RetrieveActiveContracts()

	for _, contract := range allActiveContracts {
		activeContract := ActiveContractsAPIDisplay{
			ContractID:   contract.ID.String(),
			HostID:       contract.EnodeID.String(),
			AbleToUpload: contract.Status.UploadAbility,
			AbleToRenew:  contract.Status.RenewAbility,
			Canceled:     contract.Status.Canceled,
		}
		activeContracts = append(activeContracts, activeContract)
	}

	return
}

// SetClientSetting will config the client setting based on the value provided
// it will set the bandwidth limit, rentPayment, and ipViolation check
// By setting the rentPayment, the contract maintenance
func (client *StorageClient) SetClientSetting(setting storage.ClientSetting) (err error) {
	// making sure the entire program will only be terminated after finish the SetClientSetting
	// operation

	if err = client.tm.Add(); err != nil {
		return
	}
	defer client.tm.Done()

	// input validation
	if setting.MaxUploadSpeed < 0 || setting.MaxDownloadSpeed < 0 {
		err = fmt.Errorf("both upload speed %v and download speed %v cannot be smaller than 0",
			setting.MaxUploadSpeed, setting.MaxDownloadSpeed)
		return
	}

	// set the rent payment
	if err = client.contractManager.SetRentPayment(setting.RentPayment, client.storageHostManager); err != nil {
		return
	}

	// set upload/download (write/read) bandwidth limits
	if err = client.setBandwidthLimits(setting.MaxDownloadSpeed, setting.MaxUploadSpeed); err != nil {
		return
	}

	// set the ip violation check
	client.storageHostManager.SetIPViolationCheck(setting.EnableIPViolation)

	// update and save the persist
	client.lock.Lock()
	client.persist.MaxDownloadSpeed = setting.MaxDownloadSpeed
	client.persist.MaxUploadSpeed = setting.MaxUploadSpeed
	if err = client.saveSettings(); err != nil {
		err = fmt.Errorf("failed to save the storage client settings: %s", err.Error())
		client.lock.Unlock()
		return
	}
	client.lock.Unlock()

	// active the worker pool
	client.activateWorkerPool()

	return
}

// RetrieveClientSetting will return the current storage client setting
func (client *StorageClient) RetrieveClientSetting() (setting storage.ClientSetting) {
	maxDownloadSpeed, maxUploadSpeed, _ := client.contractManager.RetrieveRateLimit()
	setting = storage.ClientSetting{
		RentPayment:       client.contractManager.AcquireRentPayment(),
		EnableIPViolation: client.storageHostManager.RetrieveIPViolationCheckSetting(),
		MaxUploadSpeed:    maxUploadSpeed,
		MaxDownloadSpeed:  maxDownloadSpeed,
	}
	return
}

// setBandwidthLimits specifies the data upload and downloading speed limit
func (client *StorageClient) setBandwidthLimits(downloadSpeedLimit, uploadSpeedLimit int64) (err error) {
	// validation
	if uploadSpeedLimit < 0 || downloadSpeedLimit < 0 {
		return errors.New("upload/download speed limit cannot be negative")
	}

	// Update the contract settings accordingly
	if uploadSpeedLimit == 0 && downloadSpeedLimit == 0 {
		client.contractManager.SetRateLimits(0, 0, 0)
	} else {
		client.contractManager.SetRateLimits(downloadSpeedLimit, uploadSpeedLimit, DefaultPacketSize)
	}

	return nil
}

// Append will send the given data to host and return the merkle root of data
func (client *StorageClient) Append(sp storage.Peer, data []byte, hostInfo storage.HostInfo) (common.Hash, error) {
	err := client.contractManager.UploadNegotiate(sp, []storage.UploadAction{{Type: storage.UploadActionAppend, Data: data}}, hostInfo)
	return merkle.Sha256MerkleTreeRoot(data), err
}

// Download requests for a single section and returns the requested data. A Merkle proof is always requested.
func (client *StorageClient) Download(sp storage.Peer, root common.Hash, offset, length uint32, hostInfo *storage.HostInfo) ([]byte, error) {
	client.lock.Lock()
	defer client.lock.Unlock()

	req := storage.DownloadRequest{
		Sector: storage.DownloadRequestSector{
			MerkleRoot: root,
			Offset:     offset,
			Length:     length,
		},
		MerkleProof: true,
	}
	var buf bytes.Buffer
	err := client.contractManager.DownloadNegotiate(sp, &buf, req, *hostInfo)
	time.Sleep(1 * time.Second)

	return buf.Bytes(), err
}

// newDownload creates and initializes a download task based on the provided parameters from outer request
func (client *StorageClient) newDownload(params downloadParams) (*download, error) {

	// params validation.
	if params.file == nil {
		return nil, errors.New("not exist the remote file")
	}
	if params.length < 0 {
		return nil, errors.New("download length cannot be negative")
	}
	if params.offset < 0 {
		return nil, errors.New("download offset cannot be negative")
	}
	if params.offset+params.length > params.file.FileSize() {
		return nil, errors.New("download data out the boundary of the remote file")
	}

	// instantiate the download object.
	d := &download{
		completeChan:      make(chan struct{}),
		startTime:         time.Now(),
		destination:       params.destination,
		destinationString: params.destinationString,
		destinationType:   params.destinationType,
		latencyTarget:     params.latencyTarget,
		length:            params.length,
		offset:            params.offset,
		overdrive:         params.overdrive,
		dxFile:            params.file,
		priority:          params.priority,
		log:               client.log,
		memoryManager:     client.memoryManager,
	}

	// record the end time when it's done.
	d.onComplete(func(_ error) error {
		d.endTime = time.Now()
		return nil
	})

	// nothing to do
	if d.length == 0 {
		d.markComplete()
		return d, nil
	}

	// calculate which segments to download
	startSegmentIndex, startSegmentOffset := params.file.SegmentIndexByOffset(params.offset)
	endSegmentIndex, endSegmentOffset := params.file.SegmentIndexByOffset(params.offset + params.length)

	if endSegmentIndex > 0 && endSegmentOffset == 0 {
		endSegmentIndex--
	}

	// map from the host id to the index of the sector within the segment
	segmentMaps := make([]map[string]downloadSectorInfo, endSegmentIndex-startSegmentIndex+1)
	for segmentIndex := startSegmentIndex; segmentIndex <= endSegmentIndex; segmentIndex++ {
		segmentMaps[segmentIndex-startSegmentIndex] = make(map[string]downloadSectorInfo)
		sectors, err := params.file.Sectors(uint64(segmentIndex))
		if err != nil {
			return nil, err
		}
		for sectorIndex, sectorSet := range sectors {
			for _, sector := range sectorSet {

				// check that a worker should not have two sectors for the same segment
				_, exists := segmentMaps[segmentIndex-startSegmentIndex][sector.HostID.String()]
				if exists {
					client.log.Error("a worker has multiple sectors for the same segment")
				}
				segmentMaps[segmentIndex-startSegmentIndex][sector.HostID.String()] = downloadSectorInfo{
					index: uint64(sectorIndex),
					root:  sector.MerkleRoot,
				}
			}
		}
	}

	// record where to write every segment
	writeOffset := int64(0)

	// record how many segments remained after every downloading
	d.segmentsRemaining += endSegmentIndex - startSegmentIndex + 1

	// queue the downloads for each segment
	for i := startSegmentIndex; i <= endSegmentIndex; i++ {
		uds := &unfinishedDownloadSegment{
			destination:  params.destination,
			erasureCode:  params.file.ErasureCode(),
			segmentIndex: i,
			segmentMap:   segmentMaps[i-startSegmentIndex],
			segmentSize:  params.file.SegmentSize(),
			sectorSize:   params.file.SectorSize(),

			// increase target by 25ms per segment
			latencyTarget:       params.latencyTarget + (25 * time.Duration(i-startSegmentIndex)),
			needsMemory:         params.needsMemory,
			priority:            params.priority,
			completedSectors:    make([]bool, params.file.ErasureCode().NumSectors()),
			physicalSegmentData: make([][]byte, params.file.ErasureCode().NumSectors()),
			sectorUsage:         make([]bool, params.file.ErasureCode().NumSectors()),
			download:            d,
			clientFile:          params.file,
		}

		// set the offset of the segment to begin downloading
		if i == startSegmentIndex {
			uds.fetchOffset = startSegmentOffset
		} else {
			uds.fetchOffset = 0
		}

		// set the number of bytes to download the segment
		if i == endSegmentIndex && endSegmentOffset != 0 {
			uds.fetchLength = endSegmentOffset - uds.fetchOffset
		} else {
			uds.fetchLength = params.file.SegmentSize() - uds.fetchOffset
		}

		// set the writeOffset where the data be written
		uds.writeOffset = writeOffset
		writeOffset += int64(uds.fetchLength)

		uds.overdrive = uint32(params.overdrive)

		// add this segment to the segment heap, and notify the download loop a new task
		client.addSegmentToDownloadHeap(uds)
		select {
		case client.newDownloads <- struct{}{}:
		default:
		}
	}
	return d, nil
}

// createDownload performs a file download and returns the download object
func (client *StorageClient) createDownload(p storage.DownloadParameters) (*download, error) {
	dxPath, err := storage.NewDxPath(p.RemoteFilePath)
	if err != nil {
		return nil, err
	}
	entry, err := client.fileSystem.OpenDxFile(dxPath)
	if err != nil {
		return nil, err
	}

	defer entry.Close()
	defer entry.SetTimeAccess(time.Now())

	// validate download parameters.
	if p.WriteToLocalPath == "" {
		return nil, errors.New("not specified local path")
	}

	// if the parameter WriteToLocalPath is not a absolute path, set default file name
	if p.WriteToLocalPath != "" && !filepath.IsAbs(p.WriteToLocalPath) {
		if strings.Contains(p.WriteToLocalPath, "/") {
			return nil, errors.New("should specify the file name not include directory，or specify absolute path")
		}

		if home := os.Getenv("HOME"); home == "" {
			return nil, errors.New("not home env")
		}

		usr, err := user.Current()
		if err != nil {
			return nil, err
		}
		p.WriteToLocalPath = filepath.Join(usr.HomeDir, p.WriteToLocalPath)
	}

	// instantiate the file to write the downloaded data
	var dw writeDestination
	var destinationType string
	osFile, err := os.OpenFile(p.WriteToLocalPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
	if err != nil {
		return nil, err
	}
	dw = osFile
	destinationType = "file"

	// create the download object.
	snap, err := entry.Snapshot()
	if err != nil {
		return nil, fmt.Errorf("cannot create snapshot: %v", err)
	}
	d, err := client.newDownload(downloadParams{
		destination:       dw,
		destinationType:   destinationType,
		destinationString: p.WriteToLocalPath,
		file:              snap,
		latencyTarget:     25e3 * time.Millisecond,

		// always download the whole file
		length:      entry.FileSize(),
		needsMemory: true,

		// always download from 0
		offset:    0,
		overdrive: 3,
		priority:  5,
	})
	if closer, ok := dw.(io.Closer); err != nil && ok {
		closeErr := closer.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("something wrong with creating download object: %v, destination close error: %v", err, closeErr)

		}
		return nil, fmt.Errorf("get something wrong with creating download object: %v, destination close successfully", err)
	} else if err != nil {
		return nil, err
	}

	// register the func, and run it when download is done.
	d.onComplete(func(_ error) error {
		if closer, ok := dw.(io.Closer); ok {
			return closer.Close()
		}
		return nil
	})

	return d, nil
}

// NOTE: DownloadSync can directly be accessed to outer request via RPC or IPC ...
// but can not async download to http response, so DownloadAsync should not open to out.

// DownloadSync performs a file download and blocks until the download is finished.
func (client *StorageClient) DownloadSync(p storage.DownloadParameters) error {
	if err := client.tm.Add(); err != nil {
		return err
	}
	defer client.tm.Done()

	d, err := client.createDownload(p)
	if err != nil {
		return err
	}

	// display the download status
	fmt.Printf("\n\ndownloading>")
	go func() {
		for {
			time.Sleep(time.Millisecond * 500)
			fmt.Printf(">")
			if d.segmentsRemaining == 0 {
				break
			}
		}
	}()

	// block until the download has completed
	select {
	case <-d.completeChan:
		return d.Err()
	case <-client.tm.StopChan():
		return errors.New("download is shutdown")
	}
}

// DownloadAsync will perform a file download without blocking until the download is finished
func (client *StorageClient) DownloadAsync(p storage.DownloadParameters) error {
	if err := client.tm.Add(); err != nil {
		return err
	}
	defer client.tm.Done()

	_, err := client.createDownload(p)
	return err
}

// GetHostAnnouncementWithBlockHash will get the HostAnnouncements and block height through the hash of the block
func (client *StorageClient) GetHostAnnouncementWithBlockHash(blockHash common.Hash) (hostAnnouncements []types.HostAnnouncement, number uint64, errGet error) {
	precompiled := vm.PrecompiledStorageContracts
	block, err := client.ethBackend.GetBlockByHash(blockHash)
	if err != nil {
		errGet = err
		return
	}
	number = block.NumberU64()
	txs := block.Transactions()
	for _, tx := range txs {
		if tx.To() == nil {
			continue
		}
		p, ok := precompiled[*tx.To()]
		if !ok {
			continue
		}
		switch p {
		case vm.HostAnnounceTransaction:
			var hac types.HostAnnouncement
			err := rlp.DecodeBytes(tx.Data(), &hac)
			if err != nil {
				client.log.Warn("Rlp decoding error as hostAnnouncements", "err", err)
				continue
			}
			hostAnnouncements = append(hostAnnouncements, hac)
		default:
			continue
		}
	}
	return
}

// GetPaymentAddress get the account address used to sign the storage contract.
// If not configured, the first address in the local wallet will be used as the paymentAddress by default.
func (client *StorageClient) GetPaymentAddress() (common.Address, error) {
	client.lock.Lock()
	paymentAddress := client.PaymentAddress
	client.lock.Unlock()

	if paymentAddress != (common.Address{}) {
		return paymentAddress, nil
	}

	//Local node does not contain wallet
	if wallets := client.ethBackend.AccountManager().Wallets(); len(wallets) > 0 {
		//The local node does not have any wallet address yet
		if accountList := wallets[0].Accounts(); len(accountList) > 0 {
			paymentAddress := accountList[0].Address
			client.lock.Lock()
			//the first address in the local wallet will be used as the paymentAddress by default.
			client.PaymentAddress = paymentAddress
			client.lock.Unlock()
			client.log.Info("host automatically sets your wallet's first account as paymentAddress")
			return paymentAddress, nil
		}
	}
	return common.Address{}, fmt.Errorf("paymentAddress must be explicitly specified")
}

// TryToRenewOrRevise will be used to check if the contract is currently
// in the middle of the revision
func (client *StorageClient) TryToRenewOrRevise(hostID enode.ID) bool {
	return client.ethBackend.TryToRenewOrRevise(hostID)
}

// RevisionOrRenewingDone indicates that the contract finished renewing
func (client *StorageClient) RevisionOrRenewingDone(hostID enode.ID) {
	client.ethBackend.RevisionOrRenewingDone(hostID)
}

// CheckAndUpdateConnection will check the connection between client
// and host. If there are no contracts signed between the two, the
// connection will be updated from the static connection to dynamic
// connection
func (client *StorageClient) CheckAndUpdateConnection(peerNode *enode.Node) {
	client.ethBackend.CheckAndUpdateConnection(peerNode)
}

// IsContractSignedWithHost is used to check if the client has signed any contract
// with the storage host provided by the user
func (client *StorageClient) IsContractSignedWithHost(hostNode *enode.Node) bool {
	// retrieve all active storage contracts
	contracts := client.contractManager.RetrieveActiveContracts()

	// compare the host node ID with each of them, if found, return true
	// otherwise, return false
	for _, contract := range contracts {
		if contract.EnodeID == hostNode.ID() {
			return true
		}
	}
	return false
}
