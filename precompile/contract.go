// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package precompile

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

const (
	selectorLen = 4
)

type RunStatefulPrecompileFunc func(accessibleState PrecompileAccessibleState, caller common.Address, addr common.Address, input []byte, suppliedGas uint64, readOnly bool) (ret []byte, remainingGas uint64, err error)

// PrecompileAccessibleState defines the interface exposed to stateful precompile contracts
type PrecompileAccessibleState interface {
	GetStateDB() StateDB
}

// StateDB is the interface for accessing EVM state
type StateDB interface {
	GetState(common.Address, common.Hash) common.Hash
	SetState(common.Address, common.Hash, common.Hash)
	SetCode(common.Address, []byte)
}

// StatefulPrecompiledContract is the interface for executing a precompiled contract
type StatefulPrecompiledContract interface {
	// Run executes the precompiled contract. Assumes that [suppliedGas] exceeds the amount
	// returned by RequiredGas.
	Run(accessibleState PrecompileAccessibleState, caller common.Address, addr common.Address, input []byte, suppliedGas uint64, readOnly bool) (ret []byte, remainingGas uint64, err error)
	// RequiredGas returns the amount of gas required to execute this precompile on [input].
	RequiredGas(input []byte) uint64
}

// statefulPrecompileFunction defines a function implemented by a stateful precompile
type statefulPrecompileFunction struct {
	// selector is the function selector for this function
	selector []byte
	// execute is performed when this function is selected
	execute RunStatefulPrecompileFunc
	// requiredGas calculates the amount of gas consumed by this function on [input].
	requiredGas func(input []byte) uint64
}

// newStatefulPrecompileFunction creates a stateful precompile function with the given arguments
func newStatefulPrecompileFunction(selector []byte, execute RunStatefulPrecompileFunc, requiredGas func(input []byte) uint64) *statefulPrecompileFunction {
	return &statefulPrecompileFunction{
		selector:    selector,
		execute:     execute,
		requiredGas: requiredGas,
	}
}

// statefulPrecompileWithFunctionSelectors implements StatefulPrecompiledContract by using 4 byte function selectors to pass
// off responsibilities to internal execution functions.
// Note: because we only ever read from [functions] there no lock is required to make it thread-safe.
type statefulPrecompileWithFunctionSelectors struct {
	functions map[string]*statefulPrecompileFunction
}

// newStatefulPrecompileWithFunctionSelectors generates new StatefulPrecompile using [functions] as the available functions.
func newStatefulPrecompileWithFunctionSelectors(functions ...*statefulPrecompileFunction) StatefulPrecompiledContract {
	contract := &statefulPrecompileWithFunctionSelectors{functions: make(map[string]*statefulPrecompileFunction)}
	for _, function := range functions {
		_, exists := contract.functions[string(function.selector)]
		if exists {
			panic(fmt.Errorf("cannot create stateful precompile with duplicated function selector: %q", function.selector))
		}
		contract.functions[string(function.selector)] = function
	}

	return contract
}

// Run selects the function using the 4 byte function selector at the start of the input and executes the underlying function on the
// given arguments.
func (s *statefulPrecompileWithFunctionSelectors) Run(accessibleState PrecompileAccessibleState, caller common.Address, addr common.Address, input []byte, suppliedGas uint64, readOnly bool) (ret []byte, remainingGas uint64, err error) {
	if len(input) < selectorLen {
		return nil, suppliedGas, fmt.Errorf("missing function selector to precompile - input length (%d)", len(input))
	}

	selector := input[:selectorLen]
	functionInput := input[selectorLen:]
	function, ok := s.functions[string(selector)]
	if !ok {
		return nil, suppliedGas, fmt.Errorf("invalid function selector %#x", selector)
	}

	return function.execute(accessibleState, caller, addr, functionInput, suppliedGas, readOnly)
}

// RequiredGas returns the amount of gas consumed by the underlying function
func (s *statefulPrecompileWithFunctionSelectors) RequiredGas(input []byte) uint64 {
	if len(input) < selectorLen {
		return 0
	}

	selector := input[:selectorLen]
	functionInput := input[selectorLen:]
	function, ok := s.functions[string(selector)]
	if !ok {
		return 0
	}

	return function.requiredGas(functionInput)
}
