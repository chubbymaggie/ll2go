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
			pred, err := getCmpPred(inst)
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
	result, err := getResult(inst)
	if err != nil {
		return nil, errutil.Err(err)
	}
	lhs := []ast.Expr{result}
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

	// Create and return the operand.
	val := tokens[1]
	switch val.Kind {
	case lltoken.Int:
		return &ast.BasicLit{Kind: token.INT, Value: val.Val}, nil
	default:
		return nil, errutil.Newf("support for LLVM IR token kind %v not yet implemented", val.Kind)
	}
}

// parseRetInst converts the provided LLVM IR ret instruction into an equivalent
// Go return statement.
//
// Syntax:
//    ret void
//    ret <type> <val>
func parseRetInst(inst llvm.Value) (*ast.ReturnStmt, error) {
	// TODO: Make more robust by using proper parsing instead of relying on
	// tokens. The current approach is used for a proof of concept and would fail
	// for composite literals. This TODO applies to the use of tokens in all
	// functions.

	// Parse and validate tokens.
	tokens, err := getTokens(inst)
	if err != nil {
		return nil, err
	}
	if len(tokens) < 4 {
		// TODO: Remove debug output.
		inst.Dump()
		return nil, errutil.Newf("unable to parse return instruction; expected >= 4 tokens, got %d", len(tokens))
	}
	typ := tokens[1]
	if typ.Kind != lltoken.Type {
		return nil, errutil.Newf(`invalid return instruction; expected type token, got %q`, typ)
	}

	// Create and return a void return statement.
	if typ.Val == "void" {
		return &ast.ReturnStmt{}, nil
	}

	// Create and return a return statement.
	val, err := parseOperand(inst.Operand(0))
	if err != nil {
		return nil, errutil.Err(err)
	}

	ret := &ast.ReturnStmt{
		Results: []ast.Expr{val},
	}
	return ret, nil
}

// getCmpPred parses the provided comparison instruction and returns a Go token
// equivalent of the comparison predicate.
//
// Syntax:
//    <result> = icmp <pred> <type> <op1>, <op2>
func getCmpPred(inst llvm.Value) (token.Token, error) {
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

// getBrCond parses the provided branch instruction and returns its condition.
//
// Syntax:
//    br i1 <cond>, label <target_true>, label <target_false>
func getBrCond(term llvm.Value) (cond ast.Expr, err error) {
	// Parse and validate tokens.
	tokens, err := getTokens(term)
	if err != nil {
		return nil, err
	}
	if len(tokens) != 10 {
		// TODO: Remove debug output.
		term.Dump()
		return nil, errutil.Newf("unable to parse conditional branch instruction; expected 10 tokens, got %d", len(tokens))
	}

	// Create and return the condition.
	switch tok := tokens[2]; tok.Kind {
	case lltoken.KwTrue, lltoken.KwFalse, lltoken.LocalVar, lltoken.LocalID:
		//    true
		//    false
		//    %foo
		//    %42
		return getIdent(tok)
	case lltoken.Int:
		//    1
		//    0
		switch tok.Val {
		case "0":
			return ast.NewIdent("false"), nil
		case "1":
			return ast.NewIdent("true"), nil
		default:
			return nil, errutil.Newf("invalid integer value; expected boolean, got %q", tok.Val)
		}
	default:
		return nil, errutil.Newf("support for LLVM IR token kind %v not yet implemented", tok.Kind)
	}
}

// getIdent converts the provided LLVM IR token into a Go identifier.
func getIdent(tok lltoken.Token) (ident ast.Expr, err error) {
	switch tok.Kind {
	case lltoken.KwTrue, lltoken.KwFalse, lltoken.LocalVar:
		return ast.NewIdent(tok.Val), nil
	case lltoken.LocalID:
		// Translate local variable IDs (e.g. "%42") to Go identifiers by adding
		// an underscore prefix (e.g. "_42").
		name := "_" + tok.Val
		return ast.NewIdent(name), nil
	default:
		return nil, errutil.Newf("support for LLVM IR token kind %v not yet implemented", tok.Kind)
	}
}

// getResult returns the result identifier of the provided assignment operation.
//
// Syntax:
//    %foo = ...
func getResult(inst llvm.Value) (result ast.Expr, err error) {
	// Parse and validate tokens.
	tokens, err := getTokens(inst)
	if err != nil {
		return nil, errutil.Err(err)
	}
	if len(tokens) < 2 {
		return nil, errutil.Newf("unable to locate result identifier; expected >= 2 tokens, got %d", len(tokens))
	}
	if eq := tokens[1]; eq.Kind != lltoken.Equal {
		return nil, errutil.Newf("invalid assigment operation; expected '=' token, got %q", eq)
	}

	// Create and return the result identifier.
	switch tok := tokens[0]; tok.Kind {
	case lltoken.LocalVar, lltoken.LocalID:
		return getIdent(tok)
	default:
		return nil, errutil.Newf("support for LLVM IR token kind %v not yet implemented", tok.Kind)
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
