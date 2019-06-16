// Copyright 2019 DxChain, All rights reserved.
// Use of this source code is governed by an Apache
// License 2.0 that can be found in the LICENSE file.

package storageclient

import (
	"fmt"

	"github.com/DxChainNetwork/godx/accounts"

	"github.com/DxChainNetwork/godx/common"
	"github.com/DxChainNetwork/godx/storage"
)

// ActiveContractAPI is used to re-format the contract information that is going to
// be displayed on the console
type ActiveContractsAPIDisplay struct {
	ContractID   string
	HostID       string
	AbleToUpload bool
	AbleToRenew  bool
	Canceled     bool
}

// PublicStorageClientAPI defines the object used to call eligible public APIs
// are used to acquire information
type PublicStorageClientAPI struct {
	sc *StorageClient
}

// NewPublicStorageClientAPI initialize PublicStorageClientAPI object
// which implemented a bunch of API methods
func NewPublicStorageClientAPI(sc *StorageClient) *PublicStorageClientAPI {
	return &PublicStorageClientAPI{sc}
}

// StorageClientSetting will retrieve the current storage client settings
func (api *PublicStorageClientAPI) StorageClientSetting() (setting storage.ClientSettingAPIDisplay) {
	return formatClientSetting(api.sc.RetrieveClientSetting())
}

// MemoryAvailable returns current memory available
func (api *PublicStorageClientAPI) MemoryAvailable() uint64 {
	return api.sc.memoryManager.MemoryAvailable()
}

// MemoryLimit returns max memory allowed
func (api *PublicStorageClientAPI) MemoryLimit() uint64 {
	return api.sc.memoryManager.MemoryLimit()
}

//GetPaymentAddress get the account address used to sign the storage contract. If not configured, the first address in the local wallet will be used as the paymentAddress by default.
func (api *PublicStorageClientAPI) GetPaymentAddress() (common.Address, error) {
	api.sc.lock.Lock()
	paymentAddress := api.sc.PaymentAddress
	api.sc.lock.Unlock()

	if paymentAddress != (common.Address{}) {
		return paymentAddress, nil
	}

	//Local node does not contain wallet
	if wallets := api.sc.ethBackend.AccountManager().Wallets(); len(wallets) > 0 {
		//The local node does not have any wallet address yet
		if accounts := wallets[0].Accounts(); len(accounts) > 0 {
			paymentAddress := accounts[0].Address
			api.sc.lock.Lock()
			//the first address in the local wallet will be used as the paymentAddress by default.
			api.sc.PaymentAddress = paymentAddress
			api.sc.lock.Unlock()
			api.sc.log.Info("host automatically sets your wallet's first account as paymentAddress")
			return paymentAddress, nil
		}
	}
	return common.Address{}, fmt.Errorf("paymentAddress must be explicitly specified")
}

// PrivateStorageClientAPI defines the object used to call eligible APIs
// that are used to configure settings
type PrivateStorageClientAPI struct {
	sc *StorageClient
}

// NewPrivateStorageClientAPI initialize PrivateStorageClientAPI object
// which implemented a bunch of API methods
func NewPrivateStorageClientAPI(sc *StorageClient) *PrivateStorageClientAPI {
	return &PrivateStorageClientAPI{sc}
}

// SetMemoryLimit allows user to expand or shrink the current memory limit
func (api *PrivateStorageClientAPI) SetMemoryLimit(amount uint64) string {
	return api.sc.memoryManager.SetMemoryLimit(amount)
}

// SetClientSetting will configure the client setting based on the user input data
func (api *PrivateStorageClientAPI) SetClientSetting(settings map[string]string) (resp string, err error) {
	prevClientSetting := api.sc.RetrieveClientSetting()
	var currentSetting storage.ClientSetting

	if currentSetting, err = parseClientSetting(settings, prevClientSetting); err != nil {
		err = fmt.Errorf("form contract failed, failed to parse the client settings: %s", err.Error())
		return
	}

	// if user entered any 0s for the rent payment, set them to the default rentPayment settings
	currentSetting = clientSettingGetDefault(currentSetting)

	// call set client setting methods
	if err = api.sc.SetClientSetting(currentSetting); err != nil {
		err = fmt.Errorf("failed to set the client settings: %s", err.Error())
		return
	}

	resp = fmt.Sprintf("Successfully set the storage client setting, you can use storageclient.setting() to verify")

	return
}

//SetPaymentAddress configure the account address used to sign the storage contract, which has and can only be the address of the local wallet.
func (api *PrivateStorageClientAPI) SetPaymentAddress(paymentAddress common.Address) bool {
	account := accounts.Account{Address: paymentAddress}
	_, err := api.sc.ethBackend.AccountManager().Find(account)
	if err != nil {
		api.sc.log.Error("You must set up an account owned by your local wallet!")
		return false
	}

	api.sc.lock.Lock()
	api.sc.PaymentAddress = paymentAddress
	api.sc.lock.Unlock()

	return true
}

// CancelAllContracts will cancel all contracts signed with storage client by
// marking all active contracts as canceled, not good for uploading, and not good
// for renewing
func (api *PrivateStorageClientAPI) CancelAllContracts() (resp string) {
	if err := api.sc.CancelContracts(); err != nil {
		resp = fmt.Sprintf("Failed to cancel all contracts: %s", err.Error())
		return
	}

	resp = fmt.Sprintf("All contracts are successfully canceled")
	return resp
}

// ActiveContracts will retrieve all active contracts and display their general information
func (api *PrivateStorageClientAPI) ActiveContracts() (activeContracts []ActiveContractsAPIDisplay) {
	activeContracts = api.sc.ActiveContracts()
	return
}

// ContractDetail will retrieve detailed contract information
func (api *PrivateStorageClientAPI) ContractDetail(contractID string) (detail ContractMetaDataAPIDisplay, err error) {
	// convert the string into contractID format
	var convertContractID storage.ContractID
	if convertContractID, err = storage.StringToContractID(contractID); err != nil {
		err = fmt.Errorf("the contract id provided is not valid, it must be in type of string")
		return
	}

	// get the contract detail
	contract, exists := api.sc.ContractDetail(convertContractID)
	if !exists {
		err = fmt.Errorf("the contract with %v does not exist", contractID)
		return
	}

	// format the contract meta data
	detail = formatContractMetaData(contract)

	return
}

// PublicStorageClientDebugAPI defines the object used to call eligible public APIs
// that are used to mock data
type PublicStorageClientDebugAPI struct {
	sc *StorageClient
}

// NewPublicStorageClientDebugAPI initialize NewPublicStorageClientDebugAPI object
// which implemented a bunch of API methods
func NewPublicStorageClientDebugAPI(sc *StorageClient) *PublicStorageClientDebugAPI {
	return &PublicStorageClientDebugAPI{sc}
}

// InsertActiveContracts will create some random contracts based on the amount user entered
// and inserted them into activeContracts field
func (api *PublicStorageClientDebugAPI) InsertActiveContracts(amount int) (resp string, err error) {
	// validate user input
	if amount <= 0 {
		err = fmt.Errorf("the amount you entered %v must be greater than 0", amount)
		return
	}

	// insert random active contracts
	if err = api.sc.contractManager.InsertRandomActiveContracts(amount); err != nil {
		err = fmt.Errorf("failed to insert mocked active contracts: %s", err.Error())
		return
	}

	resp = fmt.Sprintf("Successfully inserted %v mocked active contracts", amount)
	return
}
