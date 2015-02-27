package main

import (
	"go/ast"

	"github.com/mewkiz/pkg/errutil"

	"llvm.org/llvm/bindings/go/llvm"
)

// BasicBlock represents a conceptual basic block. If one statement of the basic
// block is executed all statements of the basic block are executed until the
// terminating instruction is reached which transfers control to another basic
// block.
type BasicBlock interface {
	// Name returns the name of the basic block.
	Name() string
	// Stmts returns the statements of the basic block.
	Stmts() []ast.Stmt
	// Term returns the terminator instruction of the basic block.
	Term() llvm.Value
}

// basicBlock represents a basic block in which the instructions have been
// translated to Go AST statement nodes but the terminator instruction is an
// unmodified LLVM IR value.
type basicBlock struct {
	// Basic block name.
	name string
	// Basic block instructions.
	stmts []ast.Stmt
	// Terminator instruction.
	term llvm.Value
}

// Name returns the name of the basic block.
func (bb *basicBlock) Name() string { return bb.name }

// Stmts returns the statements of the basic block.
func (bb *basicBlock) Stmts() []ast.Stmt { return bb.stmts }

// Term returns the terminator instruction of the basic block.
func (bb *basicBlock) Term() llvm.Value { return bb.term }

// parseBasicBlock converts the provided LLVM IR basic block into a basic block
// in which the instructions have been translated to Go AST statement nodes but
// the terminator instruction is an unmodified LLVM IR value.
func parseBasicBlock(llBB llvm.BasicBlock) (bb *basicBlock, err error) {
	name, err := getBBName(llBB.AsValue())
	if err != nil {
		return nil, err
	}
	bb = &basicBlock{name: name}
	for inst := llBB.FirstInstruction(); !inst.IsNil(); inst = llvm.NextInstruction(inst) {
		if inst == llBB.LastInstruction() {
			switch opcode := inst.InstructionOpcode(); opcode {
			// TODO: Check why there is no opcode in the llvm library for the
			// resume terminator instruction.
			case llvm.Ret, llvm.Br, llvm.Switch, llvm.IndirectBr, llvm.Invoke, llvm.Unreachable:
			default:
				return nil, errutil.Newf("non-terminator instruction %q at end of basic block", prettyOpcode(opcode))
			}
			bb.term = inst
			return bb, nil
		}
		stmt, err := parseInst(inst)
		if err != nil {
			return nil, err
		}
		bb.stmts = append(bb.stmts, stmt)
	}
	return nil, errutil.Newf("invalid basic block %q; contains no instructions", name)
}
