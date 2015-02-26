package main

import (
	"go/ast"

	"github.com/mewkiz/pkg/errutil"

	"llvm.org/llvm/bindings/go/llvm"
)

// basicBlock represents a basic block in which the instructions have been
// translated to Go AST statement nodes but the terminator instruction is an
// unmodified LLVM IR value.
type basicBlock struct {
	// Basic block name.
	name string
	// Basic block instructions.
	insts []ast.Stmt
	// Terminator instruction.
	term llvm.Value
}

// parseBasicBlock converts the provided LLVM IR basic block into a basic block
// in which the instructions have been translated to Go AST statement nodes but
// the terminator instruction is an unmodified LLVM IR value.
func parseBasicBlock(llBB llvm.BasicBlock) (bb *basicBlock, err error) {
	name, err := getBBName(llBB.AsValue())
	if err != nil {
		return nil, err
	}
	bb = &basicBlock{name: name}
	for llInst := llBB.FirstInstruction(); !llInst.IsNil(); llInst = llvm.NextInstruction(llInst) {
		if llInst == llBB.LastInstruction() {
			switch opcode := llInst.InstructionOpcode(); opcode {
			// TODO: Check why there is no opcode in the llvm library for the
			// resume terminator instruction.
			case llvm.Ret, llvm.Br, llvm.Switch, llvm.IndirectBr, llvm.Invoke, llvm.Unreachable:
			default:
				return nil, errutil.Newf("non-terminator instruction (opcode %v) at end of basic block", opcode)
			}
			bb.term = llInst
			return bb, nil
		}
		inst, err := parseInst(llInst)
		if err != nil {
			return nil, err
		}
		bb.insts = append(bb.insts, inst)
	}
	return nil, errutil.Newf("invalid basic block %q; contains no instructions", name)
}
