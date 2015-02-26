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
// node (a statement).
func parseInst(inst llvm.Value) (ast.Stmt, error) {
	// TODO: Remove debug output.
	fmt.Println("parseInst:")
	inst.Dump()
	fmt.Println()

	// Assignment.
	opcode := inst.InstructionOpcode()
	if name := inst.Name(); len(name) > 0 {
		// Standard Binary Operators
		switch opcode {
		case llvm.Add, llvm.FAdd:
			return parseBinOp(inst, token.ADD)
		case llvm.Sub, llvm.FSub:
			return parseBinOp(inst, token.SUB)
		case llvm.Mul, llvm.FMul:
			return parseBinOp(inst, token.MUL)
		case llvm.UDiv, llvm.SDiv, llvm.FDiv:
			// TODO: Handle signed and unsigned div separately.
			return parseBinOp(inst, token.QUO)
		case llvm.URem, llvm.SRem, llvm.FRem:
			// TODO: Handle signed and unsigned mod separately.
			return parseBinOp(inst, token.REM)
		}
	}

	return nil, errutil.Newf("support for LLVM IR instruction opcode %v not yet implemented", opcode)
}

// parseBinOp converts the provided LLVM IR binary operation into an equivalent
// Go AST node (an assignment statement with a binary expression on the right-
// hand side).
//
// Syntax:
//    <result> add <type> <op1>, <op2>
//
// References:
//    http://llvm.org/docs/LangRef.html#binary-operations
func parseBinOp(inst llvm.Value, op token.Token) (ast.Stmt, error) {
	x, err := parseOperand(inst.Operand(0))
	if err != nil {
		return nil, err
	}
	y, err := parseOperand(inst.Operand(1))
	if err != nil {
		return nil, err
	}
	name := inst.Name()
	if len(name) < 1 {
		// TODO: Remove debug output.
		inst.Dump()
		return nil, errutil.Newf("unable to locate result variable name of binary operation")
	}
	lhs := []ast.Expr{ast.NewIdent(name)}
	rhs := []ast.Expr{&ast.BinaryExpr{X: x, Op: op, Y: y}}
	return &ast.AssignStmt{Lhs: lhs, Tok: token.DEFINE, Rhs: rhs}, nil
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
