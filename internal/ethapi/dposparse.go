package ethapi

import (
	"fmt"
	"strings"

	"github.com/DxChainNetwork/godx/common"
	"github.com/DxChainNetwork/godx/common/unit"
	"github.com/DxChainNetwork/godx/consensus/dpos"
	"github.com/DxChainNetwork/godx/core/state"
	"github.com/DxChainNetwork/godx/core/types"
	"github.com/DxChainNetwork/godx/log"
	"github.com/DxChainNetwork/godx/rlp"
)

func ParseAndValidateCandidateApplyTxArgs(to common.Address, gas uint64, fields map[string]string, stateDB *state.StateDB) (*PrecompiledContractTxArgs, error) {
	// parse the candidateAddress field
	var candidateAddress common.Address
	if fromStr, ok := fields["from"]; ok {
		candidateAddress = common.HexToAddress(fromStr)
	}

	// form,  validate, and encode candidate tx data
	data, err := formAndValidateAndEncodeCandidateTxData(stateDB, candidateAddress, fields)
	if err != nil {
		return nil, err
	}

	return NewPrecompiledContractTxArgs(candidateAddress, to, data, nil, gas), nil
}

func ParseAndValidateVoteTxArgs(to common.Address, gas uint64, fields map[string]string, stateDB *state.StateDB) (*PrecompiledContractTxArgs, error) {
	// parse the delegator account address
	var delegatorAddress common.Address
	if fromStr, ok := fields["from"]; ok {
		delegatorAddress = common.HexToAddress(fromStr)
	}

	// form, validate, and encode vote tx data
	data, err := formAndValidateAndEncodeVoteTxData(stateDB, delegatorAddress, fields)
	if err != nil {
		return nil, err
	}

	return NewPrecompiledContractTxArgs(delegatorAddress, to, data, nil, gas), nil
}

func formAndValidateAndEncodeCandidateTxData(stateDB *state.StateDB, candidateAddress common.Address, fields map[string]string) ([]byte, error) {
	// form candidate tx data
	addCandidateTxData, err := formAddCandidateTxData(fields)
	if err != nil {
		return nil, err
	}

	// validate candidate tx data
	if err := dpos.CandidateTxDepositValidation(stateDB, addCandidateTxData, candidateAddress); err != nil {
		return nil, err
	}

	// candidate transaction data encoding
	return rlp.EncodeToBytes(&addCandidateTxData)
}

func formAndValidateAndEncodeVoteTxData(stateDB *state.StateDB, delegatorAddress common.Address, fields map[string]string) ([]byte, error) {
	// form the vote tx data
	voteTxData, err := formVoteTxData(fields)
	if err != nil {
		return nil, err
	}

	// voteTxData validation
	if err := dpos.VoteTxDepositValidation(stateDB, delegatorAddress, voteTxData); err != nil {
		return nil, err
	}

	// encode and return the data
	return rlp.EncodeToBytes(&voteTxData)
}

func formVoteTxData(fields map[string]string) (data types.VoteTxData, err error) {
	// get deposit
	depositStr, ok := fields["deposit"]
	if !ok {
		return types.VoteTxData{}, fmt.Errorf("failed to form voteTxData, vote deposit is not provided")
	}

	// get candidates
	candidatesStr, ok := fields["candidates"]
	if !ok {
		return types.VoteTxData{}, fmt.Errorf("failed to form voteTxData, vote candidates is not provided")
	}

	// parse candidates
	if data.Candidates, err = parseCandidates(candidatesStr); err != nil {
		return types.VoteTxData{}, err
	}

	// parse deposit
	if data.Deposit, err = unit.ParseCurrency(depositStr); err != nil {
		return types.VoteTxData{}, err
	}

	return
}

func formAddCandidateTxData(fields map[string]string) (data types.AddCandidateTxData, err error) {
	// get reward ratio
	rewardRatioStr, ok := fields["ratio"]
	if !ok {
		return types.AddCandidateTxData{}, fmt.Errorf("failed to form addCandidateTxData, candidate rewardRatio is not provided")
	}

	// get deposit
	depositStr, ok := fields["deposit"]
	if !ok {
		return types.AddCandidateTxData{}, fmt.Errorf("failed to form addCandidateTxData, candidate deposit is not provided")
	}

	// parse reward ratio
	if data.RewardRatio, err = parseRewardRatio(rewardRatioStr); err != nil {
		return types.AddCandidateTxData{}, err
	}

	// parse deposit
	if data.Deposit, err = unit.ParseCurrency(depositStr); err != nil {
		return types.AddCandidateTxData{}, err
	}

	return
}

func parseCandidates(candidates string) ([]common.Address, error) {
	// strip all white spaces
	candidates = strings.Replace(candidates, " ", "", -1)

	// convert it to a list of candidates
	candidateAddresses := strings.Split(candidates, ",")
	return candidatesValidationAndConversion(candidateAddresses)
}

// parseRewardRatio is used to convert the ratio from string to uint64
// and to validate the parsed ratio
func parseRewardRatio(ratio string) (uint64, error) {
	// convert the string to uint64
	rewardRatio, err := unit.ParseUint64(ratio, 1, "")
	if err != nil {
		return 0, ErrParseStringToUint
	}

	// validate the rewardRatio and return
	return rewardRatioValidation(rewardRatio)
}

// rewardRatioValidation is used to validate the reward ratio
func rewardRatioValidation(ratio uint64) (uint64, error) {
	// check if the reward ratio
	if ratio > dpos.RewardRatioDenominator {
		return 0, ErrInvalidAwardDistributionRatio
	}
	return ratio, nil
}

// candidatesValidationAndConversion will check the string list format candidates first
// and then convert the string list to address list
func candidatesValidationAndConversion(candidates []string) ([]common.Address, error) {
	// candidates validation
	if len(candidates) > MaxVoteCount {
		return nil, ErrBeyondMaxVoteSize
	}

	// candidates conversion
	var candidateAddresses []common.Address
	for _, candidate := range candidates {
		addr := common.HexToAddress(candidate)
		candidateAddresses = append(candidateAddresses, addr)
	}

	// return
	return candidateAddresses, nil
}

// CheckDposOperationTx checks the dpos transaction's filed
func CheckDposOperationTx(stateDB *state.StateDB, args *PrecompiledContractTxArgs) error {
	emptyHash := common.Hash{}
	switch args.To {

	// check CancelVote tx
	case common.BytesToAddress([]byte{16}):
		depositHash := stateDB.GetState(args.From, dpos.KeyVoteDeposit)
		if depositHash == emptyHash {
			log.Error("has not voted before,so can not submit cancel vote tx", "address", args.From.String())
			return ErrHasNotVote
		}
		return nil

	default:
		return ErrUnknownPrecompileContractAddress
	}
}