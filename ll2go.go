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
	"strings"

	"github.com/mewfork/dot"
	"github.com/mewkiz/pkg/errutil"
	"github.com/mewkiz/pkg/pathutil"
	"llvm.org/llvm/bindings/go/llvm"
)

var (
	// flagFuncs specifies a comma separated list of functions to decompile (e.g.
	// "foo,bar").
	flagFuncs string
	// When flagQuiet is true, suppress non-error messages.
	flagQuiet bool
)

func init() {
	flag.StringVar(&flagFuncs, "funcs", "", `Comma separated list of functions to decompile (e.g. "foo,bar").`)
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
	cmd := exec.Command("ll2dot", "-q", "-f", llPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return errutil.Err(err)
	}

	// Create temporary foo.bc file, e.g.
	//
	//    foo.ll -> foo.bc
	bcPath := fmt.Sprintf("/tmp/%s.bc", baseName)
	cmd = exec.Command("llvm-as", "-o", bcPath, llPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
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
		_ = f
	}

	return nil
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
	bbs := make(map[string]*basicBlock)
	for _, llBB := range llFunc.BasicBlocks() {
		bb, err := parseBasicBlock(llBB)
		if err != nil {
			return nil, err
		}
		bbs[bb.name] = bb
		printBB(bb)
	}

	// Perform control flow analysis.
	return restructure(graph, bbs)
}

// printBB pretty-prints the basic block to stdout.
func printBB(bb *basicBlock) {
	fset := token.NewFileSet()
	fmt.Printf("--- [ basic block %q ] ---\n", bb.name)
	printer.Fprint(os.Stdout, fset, bb.insts)
	fmt.Println()
	bb.term.Dump()
	fmt.Println()
}
