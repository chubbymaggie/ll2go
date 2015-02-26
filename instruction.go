package main

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/mewkiz/pkg/errutil"
	lltoken "github.com/mewlang/llvm/asm/token"
	"llvm.org/llvm/bindings/go/llvm"
)

// parseInst converts the provided LLVM IR instruction into an equivalent Go AST
// node (a statement).
func parseInst(inst llvm.Value) (ast.Stmt, error) {
	// TODO: Remove debug output.
	fmt.Println("parseInst:")
	fmt.Println("   nops:", inst.OperandsCount())
	inst.Dump()
	fmt.Println()

	// Assignment operation.
	//    %foo = ...
	opcode := inst.InstructionOpcode()
	if _, err := getResult(inst); err == nil {
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

		// Other Operators
		case llvm.ICmp, llvm.FCmp:
			pred, err := getPred(inst)
			if err != nil {
				return nil, errutil.Err(err)
			}
			return parseBinOp(inst, pred)
		}
	}

	return nil, errutil.Newf("support for LLVM IR instruction %q not yet implemented", prettyOpcode(opcode))
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
	name, err := getResult(inst)
	if err != nil {
		return nil, errutil.Err(err)
	}
	lhs := []ast.Expr{ast.NewIdent(name)}
	rhs := []ast.Expr{&ast.BinaryExpr{X: x, Op: op, Y: y}}
	return &ast.AssignStmt{Lhs: lhs, Tok: token.DEFINE, Rhs: rhs}, nil
}

// parseOperand converts the provided LLVM IR operand into an equivalent Go AST
// expression node (a basic literal, a composite literal or an identifier).
//
// Syntax:
//    i32 1
//    i32 %foo
func parseOperand(op llvm.Value) (ast.Expr, error) {
	// TODO: Support *BasicLit, *CompositeLit or *Ident.

	// Parse and validate tokens.
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

// getPred parses the provided comparison instruction and returns a Go token
// equivalent of the comparison predicate.
//
// Syntax:
//    <result> = icmp <pred> <type> <op1>, <op2>
func getPred(inst llvm.Value) (token.Token, error) {
	// Parse and validate tokens.
	tokens, err := getTokens(inst)
	if err != nil {
		return 0, errutil.Err(err)
	}
	if len(tokens) < 4 {
		return 0, errutil.Newf("unable to parse comparison instruction; expected >= 4 tokens, got %d", len(tokens))
	}

	// TODO: Handle signed and unsigned predicates separately.
	switch pred := tokens[3]; pred.Kind {
	// Int predicates.
	case lltoken.KwEq: // eq: equal
		return token.EQL, nil // ==
	case lltoken.KwNe: // ne: not equal
		return token.NEQ, nil // !=
	case lltoken.KwUgt: // ugt: unsigned greater than
		return token.GTR, nil // >
	case lltoken.KwUge: // uge: unsigned greater or equal
		return token.GEQ, nil // >=
	case lltoken.KwUlt: // ult: unsigned less than
		return token.LSS, nil // <
	case lltoken.KwUle: // ule: unsigned less or equal
		return token.LEQ, nil // <=
	case lltoken.KwSgt: // sgt: signed greater than
		return token.GTR, nil // >
	case lltoken.KwSge: // sge: signed greater or equal
		return token.GEQ, nil // >=
	case lltoken.KwSlt: // slt: signed less than
		return token.LSS, nil // <
	case lltoken.KwSle: // sle: signed less or equal
		return token.LEQ, nil // <=

	// Float predicates.
	case lltoken.KwOeq: // oeq: ordered and equal
		return token.EQL, nil // ==
	case lltoken.KwOgt: // ogt: ordered and greater than
		return token.GTR, nil // >
	case lltoken.KwOge: // oge: ordered and greater than or equal
		return token.GEQ, nil // >=
	case lltoken.KwOlt: // olt: ordered and less than
		return token.LSS, nil // <
	case lltoken.KwOle: // ole: ordered and less than or equal
		return token.LEQ, nil // <=
	case lltoken.KwOne: // one: ordered and not equal
		return token.NEQ, nil // !=
	case lltoken.KwOrd: // ord: ordered (no nans)
		return 0, errutil.Newf(`support for the floating point comparison predicate "ord" not yet implemented`)
	case lltoken.KwUeq: // ueq: unordered or equal
		return token.EQL, nil // ==
	case lltoken.KwUne: // une: unordered or not equal
		return token.NEQ, nil // !=
	case lltoken.KwUno: // uno: unordered (either nans)
		return 0, errutil.Newf(`support for the floating point comparison predicate "uno" not yet implemented`)

	default:
		return 0, errutil.Newf("invalid token; expected comparison predicate, got %q", pred)
	}
}

// getResult returns the name of the result variable in the provided assignment
// operation.
//
// Syntax:
//    %foo = ...
func getResult(inst llvm.Value) (name string, err error) {
	// Parse and validate tokens.
	tokens, err := getTokens(inst)
	if err != nil {
		return "", errutil.Err(err)
	}
	if len(tokens) < 2 {
		return "", errutil.Newf("unable to locate result variable name; expected >= 2 tokens, got %d", len(tokens))
	}
	if eq := tokens[1]; eq.Kind != lltoken.Equal {
		return "", errutil.Newf("invalid assigment operation; expected '=' token, got %q", eq)
	}

	switch ident := tokens[0]; ident.Kind {
	case lltoken.LocalID, lltoken.LocalVar:
		return ident.Val, nil
	default:
		return "", errutil.Newf("support for LLVM IR token kind %v not yet implemented", ident.Kind)
	}
}

// prettyOpcode returns a string representation of the given LLVM IR instruction
// opcode.
func prettyOpcode(opcode llvm.Opcode) string {
	m := map[llvm.Opcode]string{
		llvm.Ret:         "Ret",
		llvm.Br:          "Br",
		llvm.Switch:      "Switch",
		llvm.IndirectBr:  "IndirectBr",
		llvm.Invoke:      "Invoke",
		llvm.Unreachable: "Unreachable",

		// Standard Binary Operators
		llvm.Add:  "Add",
		llvm.FAdd: "FAdd",
		llvm.Sub:  "Sub",
		llvm.FSub: "FSub",
		llvm.Mul:  "Mul",
		llvm.FMul: "FMul",
		llvm.UDiv: "UDiv",
		llvm.SDiv: "SDiv",
		llvm.FDiv: "FDiv",
		llvm.URem: "URem",
		llvm.SRem: "SRem",
		llvm.FRem: "FRem",

		// Logical Operators
		llvm.Shl:  "Shl",
		llvm.LShr: "LShr",
		llvm.AShr: "AShr",
		llvm.And:  "And",
		llvm.Or:   "Or",
		llvm.Xor:  "Xor",

		// Memory Operators
		llvm.Alloca:        "Alloca",
		llvm.Load:          "Load",
		llvm.Store:         "Store",
		llvm.GetElementPtr: "GetElementPtr",

		// Cast Operators
		llvm.Trunc:    "Trunc",
		llvm.ZExt:     "ZExt",
		llvm.SExt:     "SExt",
		llvm.FPToUI:   "FPToUI",
		llvm.FPToSI:   "FPToSI",
		llvm.UIToFP:   "UIToFP",
		llvm.SIToFP:   "SIToFP",
		llvm.FPTrunc:  "FPTrunc",
		llvm.FPExt:    "FPExt",
		llvm.PtrToInt: "PtrToInt",
		llvm.IntToPtr: "IntToPtr",
		llvm.BitCast:  "BitCast",

		// Other Operators
		llvm.ICmp:           "ICmp",
		llvm.FCmp:           "FCmp",
		llvm.PHI:            "PHI",
		llvm.Call:           "Call",
		llvm.Select:         "Select",
		llvm.VAArg:          "VAArg",
		llvm.ExtractElement: "ExtractElement",
		llvm.InsertElement:  "InsertElement",
		llvm.ShuffleVector:  "ShuffleVector",
		llvm.ExtractValue:   "ExtractValue",
		llvm.InsertValue:    "InsertValue",
	}

	s, ok := m[opcode]
	if !ok {
		return fmt.Sprintf("<unknown opcode %d>", int(opcode))
	}
	return s
}
