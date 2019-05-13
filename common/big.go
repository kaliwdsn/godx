// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package common

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// Common big integers often used
var (
	Big1   = big.NewInt(1)
	Big2   = big.NewInt(2)
	Big3   = big.NewInt(3)
	Big0   = big.NewInt(0)
	Big32  = big.NewInt(32)
	Big256 = big.NewInt(256)
	Big257 = big.NewInt(257)
)

var (
	BigInt0 = NewBigInt(0)
	BigInt1 = NewBigInt(1)
)

type BigInt struct {
	b big.Int
}

func NewBigInt(x int64) BigInt {
	return BigInt{
		b: *big.NewInt(x),
	}
}

func NewBigIntUint64(x uint64) BigInt {
	return BigInt {
		b: *new(big.Int).SetUint64(x),
	}
}

func NewBigIntFloat64(x float64) BigInt {
	v := uint64(x)
	return BigInt {
		b: *new(big.Int).SetUint64(v),
	}
}

func RandomBigIntRange(x BigInt) (random BigInt, err error) {

	randint, err := rand.Int(rand.Reader, x.BigIntPtr())

	if err != nil {
		return BigInt{}, err
	}
	random = BigInt{
		b: *randint,
	}
	return
}

func RandomBigInt() BigInt {
	randint, _ := rand.Int(rand.Reader, big.NewInt(1000))
	return BigInt{
		b: *randint,
	}
}

func (x BigInt) String() string {
	return x.b.String()
}

func (x BigInt) IsNeg() bool {
	if x.Cmp(NewBigInt(0)) < 0 {
		return true
	}
	return false
}

func (x BigInt) Add(y BigInt) (sum BigInt) {
	sum.b.Add(&x.b, &y.b)
	return
}

func (x BigInt) Sub(y BigInt) (diff BigInt) {
	diff.b.Sub(&x.b, &y.b)
	return
}

func (x BigInt) Mult(y BigInt) (prod BigInt) {
	prod.b.Mul(&x.b, &y.b)
	return
}

func (x BigInt) Cmp(y BigInt) (result int) {
	result = x.b.Cmp(&y.b)
	return
}

func (x BigInt) MultInt(y int64) (prod BigInt) {
	prod.b.Mul(&x.b, big.NewInt(y))
	return
}

func (x BigInt) Div(y BigInt) (quotient BigInt) {
	// denominator cannot be 0
	if y.Cmp(NewBigInt(0)) == 0 {
		y = NewBigInt(1)
	}

	// division
	quotient.b.Div(&x.b, &y.b)
	return
}

func (x BigInt) DivUint64(y uint64) (quotient BigInt) {
	quotient.b.Div(&x.b, new(big.Int).SetUint64(y))
	return
}

func (x BigInt) Float64() (result float64) {
	f := new(big.Float).SetInt(&x.b)
	result, _ = f.Float64()
	return
}

func (x BigInt) BigIntPtr() *big.Int {
	return &x.b
}

func (x BigInt) MultUint64(y uint64) (prod BigInt) {
	prod.b.Mul(&x.b, new(big.Int).SetUint64(y))
	return
}

func (x BigInt) MultFloat64(y float64) (prod BigInt) {
	xRat := new(big.Rat).SetInt(&x.b)
	yRat := new(big.Rat).SetFloat64(y)
	ratProd := new(big.Rat).Mul(xRat, yRat)
	prod.b.Div(ratProd.Num(), ratProd.Denom())
	return
}

func (x BigInt) MarshalJSON() ([]byte, error) {
	return []byte(x.b.String()), nil
}

func (x *BigInt) UnmarshalJSON(val []byte) error {
	if string(val) == "null" {
		return nil
	}
	var y big.Int
	_, ok := y.SetString(string(val), 10)
	if !ok {
		return fmt.Errorf("invalid big integer: %s", y)
	}
	x.b = y
	return nil
}
