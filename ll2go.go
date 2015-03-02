//go:generate mango -plain ll2go.go

// ll2go is a tool which decompiles LLVM IR assembly files to Go source code
// (e.g. *.ll -> *.go).
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mewfork/dot"
	"github.com/mewkiz/pkg/errutil"
	"github.com/mewkiz/pkg/osutil"
	"github.com/mewkiz/pkg/pathutil"
	"llvm.org/llvm/bindings/go/llvm"
)

var (
	// When flagForce is true, force overwrite existing Go source code.
	flagForce bool
	// flagFuncs specifies a comma separated list of functions to decompile (e.g.
	// "foo,bar").
	flagFuncs string
	// flagPkgName specifies the package name if non-empty.
	flagPkgName string
	// When flagQuiet is true, suppress non-error messages.
	flagQuiet bool
)

func init() {
	flag.BoolVar(&flagForce, "f", false, "Force overwrite existing Go source code.")
	flag.StringVar(&flagFuncs, "funcs", "", `Comma separated list of functions to decompile (e.g. "foo,bar").`)
	flag.StringVar(&flagPkgName, "pkgname", "", "Package name.")
	flag.BoolVar(&flagQuiet, "q", false, "Suppress non-error messages.")
	flag.Usage = usage
}

const use = `
Usage: ll2go [OPTION]... FILE...
Decompile LLVM IR assembly files to Go source code (e.g. *.ll -> *.go).

Flags:`

func usage() {
	fmt.Fprintln(os.Stderr, use[1:])
	flag.PrintDefaults()
}

func main() {
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}
	for _, llPath := range flag.Args() {
		err := ll2go(llPath)
		if err != nil {
			log.Fatalln(err)
		}
	}
}

// ll2go parses the provided LLVM IR assembly file and decompiles it to Go
// source code.
func ll2go(llPath string) error {
	// File name and file path without extension.
	baseName := pathutil.FileName(llPath)
	basePath := pathutil.TrimExt(llPath)

	// TODO: Create graphs in /tmp/xxx_graphs/*.dot

	// Create temporary foo.dot file, e.g.
	//
	//    foo.ll -> foo_graphs/*.dot
	dotDir := basePath + "_graphs"
	if ok, _ := osutil.Exists(dotDir); !ok {
		if !flagQuiet {
			log.Printf("Creating control flow graphs for %q.\n", filepath.Base(llPath))
		}
		cmd := exec.Command("ll2dot", "-q", "-f", llPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			return errutil.Err(err)
		}
	}

	// Create temporary foo.bc file, e.g.
	//
	//    foo.ll -> foo.bc
	bcPath := fmt.Sprintf("/tmp/%s.bc", baseName)
	cmd := exec.Command("llvm-as", "-o", bcPath, llPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return errutil.Err(err)
	}

	// Remove temporary foo.bc file.
	defer func() {
		err = os.Remove(bcPath)
		if err != nil {
			log.Fatalln(errutil.Err(err))
		}
	}()

	// Parse foo.bc
	module, err := llvm.ParseBitcodeFile(bcPath)
	if err != nil {
		return errutil.Err(err)
	}
	defer module.Dispose()

	// Get function names.
	var funcNames []string
	if len(flagFuncs) > 0 {
		// Get function names from command line flag:
		//
		//    -funcs="foo,bar"
		funcNames = strings.Split(flagFuncs, ",")
	} else {
		// Get all function names.
		for llFunc := module.FirstFunction(); !llFunc.IsNil(); llFunc = llvm.NextFunction(llFunc) {
			if llFunc.IsDeclaration() {
				// Ignore function declarations (e.g. functions without bodies).
				continue
			}
			funcNames = append(funcNames, llFunc.Name())
		}
	}

	// Locate package name.
	pkgName := flagPkgName
	if len(flagPkgName) == 0 {
		pkgName = baseName
		for _, funcName := range funcNames {
			if funcName == "main" {
				pkgName = "main"
				break
			}
		}
	}

	// Create foo.go.
	file := &ast.File{
		Name: ast.NewIdent(pkgName),
	}

	// TODO: Implement support for global variables.

	// Parse each function.
	for _, funcName := range funcNames {
		if !flagQuiet {
			log.Printf("Parsing function: %q\n", funcName)
		}
		graph, err := parseCFG(basePath, funcName)
		if err != nil {
			return errutil.Err(err)
		}
		f, err := parseFunc(graph, module, funcName)
		if err != nil {
			return errutil.Err(err)
		}
		fmt.Printf("=== [ function %q ] ===\n", funcName)
		fmt.Println()
		printFunc(f)
		fmt.Println()
		file.Decls = append(file.Decls, f)
	}

	goName := baseName + ".go"
	printFile(goName, file)

	// Store Go source code to file.
	goPath := basePath + ".go"
	if !flagQuiet {
		log.Printf("Creating: %q\n", goPath)
	}
	return storeFile(goPath, file)
}

// parseCFG parses the control flow graph of the function.
//
// For a source file "foo.ll" containing the functions "bar" and "baz" the
// following DOT files will be created:
//
//    foo_graphs/bar.dot
//    foo_graphs/baz.dot
func parseCFG(basePath, funcName string) (graph *dot.Graph, err error) {
	dotDir := basePath + "_graphs"
	dotName := funcName + ".dot"
	dotPath := fmt.Sprintf("%s/%s", dotDir, dotName)
	return dot.ParseFile(dotPath)
}

// parseFunc parses the given function and attempts to construct an equivalent
// Go function declaration AST node.
func parseFunc(graph *dot.Graph, module llvm.Module, funcName string) (*ast.FuncDecl, error) {
	llFunc := module.NamedFunction(funcName)
	if llFunc.IsNil() {
		return nil, errutil.Newf("unable to locate function %q", funcName)
	}
	if llFunc.IsDeclaration() {
		return nil, errutil.Newf("unable to create AST for %q; expected function definition, got function declaration (e.g. no body)", funcName)
	}

	// Parse each basic block.
	bbs := make(map[string]BasicBlock)
	for _, llBB := range llFunc.BasicBlocks() {
		bb, err := parseBasicBlock(llBB)
		if err != nil {
			return nil, err
		}
		bbs[bb.Name()] = bb
		printBB(bb)
	}

	// Replace PHI instructions with assignment statements in the appropriate
	// basic blocks.
	for _, bb := range bbs {
		block, ok := bb.(*basicBlock)
		if !ok {
			return nil, errutil.Newf("invalid basic block type; expected *basicBlock, got %T", bb)
		}
		for ident, defs := range block.phis {
			fmt.Println("block:", block.Name())
			fmt.Println("  ident:", ident)
			fmt.Println("  defs: ", defs)
			for _, def := range defs {
				assign := &ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent(ident)},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{def.expr},
				}
				bbSrc := bbs[def.bb]
				stmts := bbSrc.Stmts()
				stmts = append(stmts, assign)
				bbSrc.SetStmts(stmts)
			}
		}
	}

	// Perform control flow analysis.
	body, err := restructure(graph, bbs)
	if err != nil {
		return nil, errutil.Err(err)
	}
	sig := &ast.FuncType{
		Params: &ast.FieldList{},
	}
	if funcName != "main" {
		// TODO: Implement parsing of function signature.
	}
	return createFunc(funcName, sig, body)
}

// createFunc creates and returns a Go function declaration based on the
// provided function name, function signature and basic block.
func createFunc(name string, sig *ast.FuncType, body *ast.BlockStmt) (*ast.FuncDecl, error) {
	f := &ast.FuncDecl{
		Name: ast.NewIdent(name),
		Type: sig,
		Body: body,
	}
	return f, nil
}

// storeFile stores the given Go source code to the provided file path.
func storeFile(goPath string, file *ast.File) error {
	// Don't force overwrite Go output file.
	if !flagForce {
		if ok, _ := osutil.Exists(goPath); ok {
			return errutil.Newf("output file %q already exists", goPath)
		}
	}
	f, err := os.Create(goPath)
	if err != nil {
		return err
	}
	defer f.Close()
	fset := token.NewFileSet()
	return printer.Fprint(f, fset, file)
}

// printBB pretty-prints the basic block to stdout.
func printBB(bb BasicBlock) {
	fset := token.NewFileSet()
	fmt.Printf("--- [ basic block %q ] ---\n", bb.Name())
	printer.Fprint(os.Stdout, fset, bb.Stmts())
	fmt.Println()
	if term := bb.Term(); !term.IsNil() {
		term.Dump()
	}
	fmt.Println()
}

// printFunc pretty-prints the function to stdout.
func printFunc(f *ast.FuncDecl) {
	fset := token.NewFileSet()
	fmt.Printf("--- [ function %q ] ---\n", f.Name)
	printer.Fprint(os.Stdout, fset, f)
	fmt.Println()
}

// printFile pretty-prints the Go file to stdout.
func printFile(name string, file *ast.File) {
	fset := token.NewFileSet()
	fmt.Printf("--- [ file %q ] ---\n", name)
	printer.Fprint(os.Stdout, fset, file)
	fmt.Println()
}
