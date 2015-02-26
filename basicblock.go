package main

import (
	"go/ast"

	"llvm.org/llvm/bindings/go/llvm"
)

// parseBasicBlock converts the provided LLVM IR basic block into an equivalent
// Go AST node.
func parseBasicBlock(bb llvm.BasicBlock) (node *ast.Node, err error) {
	for inst := bb.FirstInstruction(); !inst.IsNil(); inst = llvm.NextInstruction(inst) {
		stmt, err := parseInst(inst)
		if err != nil {
			return nil, err
		}
		_ = stmt
	}
	panic("not yet implemented")
}
