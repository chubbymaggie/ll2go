package main

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/mewkiz/pkg/errutil"
	lltoken "github.com/mewlang/llvm/asm/token"
	"llvm.org/llvm/bindings/go/llvm"
)

// NOTE: Start with one of everything rather than supporting every binary op.
//    1) add <type> <op1> <op2>            ->   *ast.BinaryExpr
//    2) <result> add <type> <op1> <op2>   ->   *ast.AssignStmt

// parseInst converts the provided LLVM IR instruction into an equivalent Go AST
// node.
func parseInst(inst llvm.Value) (ast.Node, error) {
	// TODO: Remove debug output.
	fmt.Println("parseInst:")
	inst.Dump()
	fmt.Println()

	// Standard Binary Operators
	var expr ast.Expr
	opcode := inst.InstructionOpcode()
	switch opcode {
	case llvm.Add:
		var err error
		expr, err = parseAddInst(inst)
		if err != nil {
			return nil, err
		}
	//case llvm.FAdd:
	//case llvm.Sub:
	//case llvm.FSub:
	//case llvm.Mul:
	//case llvm.FMul:
	//case llvm.UDiv:
	//case llvm.SDiv:
	//case llvm.FDiv:
	//case llvm.URem:
	//case llvm.SRem:
	//case llvm.FRem:
	default:
		return nil, errutil.Newf("support for LLVM IR instruction opcode %v not yet implemented", opcode)
	}

	// Assignment?
	fmt.Println("name:", inst.Name())

	return expr, nil
}

// parseAddInst converts the provided LLVM IR add instruction into an equivalent
// Go AST node (a binary expression).
//
// Syntax:
//    add <type> <op1>, <op2>
func parseAddInst(inst llvm.Value) (ast.Expr, error) {
	x, err := parseOperand(inst.Operand(0))
	if err != nil {
		return nil, err
	}
	y, err := parseOperand(inst.Operand(1))
	if err != nil {
		return nil, err
	}
	return &ast.BinaryExpr{X: x, Op: token.ADD, Y: y}, nil
}

// parseOperand converts the provided LLVM IR operand into an equivalent Go AST
// expression node (a basic literal, a composite literal or an identifier).
func parseOperand(op llvm.Value) (ast.Expr, error) {
	// TODO: Support *BasicLit, *CompositeLit or *Ident.

	tokens, err := getTokens(op)
	if err != nil {
		return nil, err
	}
	if len(tokens) != 3 {
		// TODO: Remove debug output.
		op.Dump()
		return nil, errutil.Newf("unable to parse operand; expected 3 tokens, got %d", len(tokens))
	}
	// TODO: Add support for operand of other types than int.

	// Syntax:
	//    i32 1
	//    i32 %foo

	// TODO: Parse type.
	//typ := tokens[0]

	val := tokens[1]
	switch val.Kind {
	case lltoken.Int:
		return &ast.BasicLit{Kind: token.INT, Value: val.Val}, nil
	default:
		return nil, errutil.Newf("support for LLVM IR token kind %v not yet implemented", val.Kind)
	}
}
