package main

import (
	"go/ast"

	"llvm.org/llvm/bindings/go/llvm"
)

// parseBasicBlock parses the provided LLVM IR basic block and attempts to
// construct an equivalent Go AST node.
func parseBasicBlock(bb llvm.BasicBlock) (node *ast.Node, err error) {
	for inst := bb.FirstInstruction(); !inst.IsNil(); inst = llvm.NextInstruction(inst) {
		stmt, err := parseInst(inst)
		if err != nil {
			return nil, err
		}
		_ = stmt
	}
	panic("not yet implemented.")
}

// parseInst parses the provided LLVM IR instruction and attempts to convert it
// to an equivalent Go statement AST node.
func parseInst(inst llvm.Value) (stmt *ast.Stmt, err error) {
	panic("not yet implemented.")
}
