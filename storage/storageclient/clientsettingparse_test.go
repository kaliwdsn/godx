// Copyright 2019 DxChain, All rights reserved.
// Use of this source code is governed by an Apache
// License 2.0 that can be found in the LICENSE file

package storageclient

import (
	"github.com/DxChainNetwork/godx/common"
	"github.com/DxChainNetwork/godx/storage"
	"math/big"
	"testing"
)

func TestContainsDigitOnly(t *testing.T) {
	tables := []struct {
		input     string
		allDigits bool
	}{
		{"1231232131232131231231321313", true},
		{"fjsdlkfjalkfjlkahdfklhfiwehflsdjfkjf", false},
		{"21rfewf2rewf4r4", false},
		{"fdafasdf", false},
	}

	for _, table := range tables {
		result := containsDigitOnly(table.input)
		if result != table.allDigits {
			t.Errorf("the string %s is expected allcontaindigit %t, got %t",
				table.input, table.allDigits, result)
		}
	}
}

func TestParseFund(t *testing.T) {
	tables := []struct {
		fund   string
		result common.BigInt
	}{
		{"100 h", common.NewBigInt(100)},
		{"100 DX", fundUnitConversion(currencyIndexMap["dx"], big.NewInt(100))},
		{"100 GDX", fundUnitConversion(currencyIndexMap["Gdx"], big.NewInt(100))},
		{"100  nDx", fundUnitConversion(currencyIndexMap["ndx"], big.NewInt(100))},
		{"99876Kdx", fundUnitConversion(currencyIndexMap["Kdx"], big.NewInt(99876))},
	}

	for _, table := range tables {

		// parse the fund
		parsed, err := parseFund(table.fund)
		if err != nil {
			t.Fatalf("failed to parse the fund %s: %s", table.fund, err.Error())
		}

		// parsed result verification
		if !parsed.IsEqual(table.result) {
			t.Errorf("the parsed value does not match with the expected result: expected %+v, got %+v",
				table.result, parsed)
		}
	}
}

func TestParseFundFail(t *testing.T) {
	var failedExpected = []string{
		"100dxc",   // does not match with any unit
		"100ldx",   // error, even the suffix is dx
		"120lgdx",  // error, even the suffix is dx
		"a1200gdx", // error, even the suffix is dx
		"120gdx",   // lower case g, does not match with any unit
		"120kdx",   // lower case k, does not match with any unit
	}

	for _, failedCase := range failedExpected {
		if _, err := parseFund(failedCase); err == nil {
			t.Errorf("error is expected with the input %s", failedCase)
		}

	}
}

func TestParseStorageHosts(t *testing.T) {
	var tables = []struct {
		hosts  string
		parsed uint64
	}{
		{"1231231", 1231231},
		{"34123431324", 34123431324},
		{"023123131", 23123131},
	}

	for _, table := range tables {
		result, err := parseStorageHosts(table.hosts)
		if err != nil {
			t.Fatalf("failed to parse the storage hosts: %s", err.Error())
		}

		if result != table.parsed {
			t.Errorf("by using %s as input, expected parsed value %v, got %v",
				table.hosts, table.parsed, result)
		}
	}
}

func TestParsePeriodRenew(t *testing.T) {
	var tables = []struct {
		period string
		parsed uint64
		err    bool
	}{
		{"100 d", 100 * storage.BlocksPerDay, false},
		{"182 h", 182 * storage.BlockPerHour, false},
		{"179 W", 179 * storage.BlocksPerWeek, false},
		{"3000 M", 3000 * storage.BlocksPerMonth, false},
		{"10 y", 10 * storage.BlocksPerYear, false},
		{"10000 b", 10000, false},
		{"10000 J", 0, true},
		{"100u0 d", 0, true},
	}

	for _, table := range tables {
		result, err := parsePeriodAndRenew(table.period)
		if err != nil && table.err {
			continue
		}

		if err != nil {
			t.Fatalf("error is not expected, got error: %s", err.Error())
		}

		if result != table.parsed {
			t.Errorf("input %+v, epxected parsed value %+v, got %+v",
				table.period, table.parsed, result)
		}
	}
}
