package main

import (
	"fmt"
	"go/ast"
	"log"
	"path/filepath"
	"sort"

	"llvm.org/llvm/bindings/go/llvm"

	"github.com/mewfork/dot"
	"github.com/mewkiz/pkg/errutil"
	"github.com/mewkiz/pkg/goutil"
	"github.com/mewrev/graphs"
	"github.com/mewrev/graphs/iso"
	"github.com/mewrev/graphs/merge"
)

var (
	// subs is an ordered list of subgraphs representing common control-flow
	// primitives such as 2-way conditionals, pre-test loops, etc.
	subs []*graphs.SubGraph
	// subNames specifies the name of each subgraph in subs, arranged in the same
	// order.
	subNames = []string{
		"list.dot", "if.dot", "if_else.dot", "pre_loop.dot", "post_loop.dot",
		"if_return.dot",
	}
)

func init() {
	// Parse subgraphs representing common control flow primitives.
	subDir, err := goutil.SrcDir("github.com/mewrev/graphs/testdata/primitives")
	if err != nil {
		log.Fatalln(errutil.Err(err))
	}
	for _, subName := range subNames {
		subPath := filepath.Join(subDir, subName)
		sub, err := graphs.ParseSubGraph(subPath)
		if err != nil {
			log.Fatalln(errutil.Err(err))
		}
		subs = append(subs, sub)
	}
}

// primitive represents a control flow primitive, such as a 2-way conditional, a
// pre-test loop or simply a single statement or a list of statements.
type primitive struct {
	// The control flow primitive is conceptually a basic block, and as such
	// requires a basic block name.
	name string
	// Statements of the control flow primitive.
	stmts []ast.Stmt
	// Terminator instruction.
	term llvm.Value
}

// restructure attempts to create a structured control flow for a function based
// on the provided graph (which contains one node per basic block) and the slice
// of basic blocks. It does so by repeatedly locating and merging structured
// subgraphs into single nodes until the entire graph is reduced into a single
// node or no structured subgraphs may be located.
func restructure(graph *dot.Graph, funcBBs map[string]*basicBlock) (*ast.FuncDecl, error) {
	funcPrims := make(map[string]*primitive)
	for {
		if len(funcBBs) <= 1 {
			fmt.Println("restructure: DONE :)")
			fmt.Println("   funcBBs:", funcBBs)
			fmt.Println("   funcPrims:", funcPrims)
			fmt.Println()
			break
		}
		prim, err := locatePrim(graph, funcBBs, funcPrims)
		if err != nil {
			return nil, errutil.Err(err)
		}
		fmt.Println("located primitive:", prim)
		funcPrims[prim.name] = prim
	}
	panic("not yet implemented")
}

// locatePrim locates a control flow primitive in the provided function's
// control flow graph and its basic blocks.
func locatePrim(graph *dot.Graph, funcBBs map[string]*basicBlock, funcPrims map[string]*primitive) (prim *primitive, err error) {
	for i, sub := range subs {
		// Locate an isomorphism of sub in graph.
		m, ok := iso.Search(graph, sub)
		if !ok {
			// No match, try next control flow primitive.
			continue
		}
		printMapping(graph, sub, m)

		// Merge the nodes of the subgraph isomorphism into a single node.
		newName, err := merge.Merge(graph, m, sub)
		if err != nil {
			return nil, errutil.Err(err)
		}

		// Create a control flow primitive based on the identified subgraph.
		primBBs := make(map[string]*basicBlock)
		primPrims := make(map[string]*primitive)
		for _, gname := range m {
			bb, ok := funcBBs[gname]
			if ok {
				primBBs[gname] = bb
				delete(funcBBs, gname)
				continue
			}
			prim, ok := funcPrims[gname]
			if ok {
				primPrims[gname] = prim
				delete(funcPrims, gname)
				continue
			}
			return nil, errutil.Newf("unable to locate basic block %q", gname)
		}
		subName := subNames[i]
		return createPrim(subName, m, primBBs, primPrims, newName)
	}

	return nil, errutil.New("unable to locate control flow primitive")
}

// createPrim creates a control flow primitive based on the identified subgraph,
// its node pair mapping, and its basic block (and primitives forming conceptual
// basic blocks). The new control flow primitive conceptually forms a new basic
// block with the specified name.
func createPrim(subName string, m map[string]string, bbs map[string]*basicBlock, prims map[string]*primitive, newName string) (prim *primitive, err error) {
	switch subName {
	case "if.dot":
		return createIfPrim(m, bbs, prims, newName)
	//case "if_else.dot":
	//case "if_return.dot":
	case "list.dot":
		return createListPrim(m, bbs, prims, newName)
	//case "post_loop.dot":
	//case "pre_loop.dot":
	default:
		return nil, errutil.Newf("control flow primitive of subgraph %q not yet supported", subName)
	}
}

// createListPrim creates a list primitive containing a slice of Go statements
// based on the identified subgraph, its node pair mapping, and its basic block
// (and primitives forming conceptual basic blocks). The new control flow
// primitive conceptually forms a new basic block with the specified name.
//
// Contents of "list.dot":
//
//    digraph list {
//       A [label="entry"]
//       B [label="exit"]
//       A->B
//    }
func createListPrim(m map[string]string, bbs map[string]*basicBlock, prims map[string]*primitive, newName string) (prim *primitive, err error) {
	// Locate graph node names.
	nameA, ok := m["A"]
	if !ok {
		return nil, errutil.New(`unable to locate node pair for sub node "A"`)
	}
	nameB, ok := m["B"]
	if !ok {
		return nil, errutil.New(`unable to locate node pair for sub node "B"`)
	}

	// Locate the statments of the entry ("A") and exit ("B") basic blocks (or
	// primitives).
	stmtsA, err := getStatements(bbs, prims, nameA)
	if err != nil {
		return nil, errutil.Err(err)
	}
	stmtsB, err := getStatements(bbs, prims, nameB)
	if err != nil {
		return nil, errutil.Err(err)
	}

	// Locate the teminator instruction of the exit ("B") basic block (or
	// primitive).
	termB, err := getTerminator(bbs, prims, nameB)
	if err != nil {
		return nil, errutil.Err(err)
	}

	// Create and return new primitive.
	stmts := append(stmtsA, stmtsB...)
	prim = &primitive{
		name:  newName,
		stmts: stmts,
		term:  termB,
	}
	return prim, nil
}

// createIfPrim creates an if-statement primitive based on the identified
// subgraph, its node pair mapping, and its basic block (and primitives forming
// conceptual basic blocks). The new control flow primitive conceptually forms a
// new basic block with the specified name.
//
// Contents of "if.dot":
//
//    digraph if {
//       A [label="entry"]
//       B
//       C [label="exit"]
//       A->B [label="true"]
//       A->C [label="false"]
//       B->C
//    }
func createIfPrim(m map[string]string, bbs map[string]*basicBlock, prims map[string]*primitive, newName string) (prim *primitive, err error) {
	// Locate graph node names.
	nameA, ok := m["A"]
	if !ok {
		return nil, errutil.New(`unable to locate node pair for sub node "A"`)
	}
	nameB, ok := m["B"]
	if !ok {
		return nil, errutil.New(`unable to locate node pair for sub node "B"`)
	}
	nameC, ok := m["C"]
	if !ok {
		return nil, errutil.New(`unable to locate node pair for sub node "C"`)
	}

	// Locate the statements of the cond ("A") and body ("B") basic blocks (or
	// primitives).
	stmtsA, err := getStatements(bbs, prims, nameA)
	if err != nil {
		return nil, errutil.Err(err)
	}
	stmtsB, err := getStatements(bbs, prims, nameB)
	if err != nil {
		return nil, errutil.Err(err)
	}
	stmtsC, err := getStatements(bbs, prims, nameC)
	if err != nil {
		return nil, errutil.Err(err)
	}

	// Locate the teminator instruction of the cond ("A") and exit ("C") basic
	// blocks (or primitives).
	termA, err := getTerminator(bbs, prims, nameA)
	if err != nil {
		return nil, errutil.Err(err)
	}
	termC, err := getTerminator(bbs, prims, nameC)
	if err != nil {
		return nil, errutil.Err(err)
	}

	// Create and return new primitive.
	fmt.Println("### [ cond ] ###")
	termA.Dump()
	fmt.Println()
	cond, err := getBrCond(termA)
	if err != nil {
		return nil, errutil.Err(err)
	}
	body := &ast.BlockStmt{
		List: stmtsB,
	}
	ifStmt := &ast.IfStmt{
		Cond: cond,
		Body: body,
	}
	stmts := append(stmtsA, ifStmt)
	stmts = append(stmts, stmtsC...)
	prim = &primitive{
		name:  newName,
		stmts: stmts,
		term:  termC,
	}
	return prim, nil
}

// getStatements returns the list of statements from the named basic block (or
// primitive forming a conceptual basic block).
func getStatements(bbs map[string]*basicBlock, prims map[string]*primitive, name string) ([]ast.Stmt, error) {
	// Get statements from basic block.
	if bb, ok := bbs[name]; ok {
		return bb.insts, nil
	}

	// Get statements from primitive.
	if prim, ok := prims[name]; ok {
		return prim.stmts, nil
	}

	return nil, errutil.Newf("unable to locate basic block (or primitive forming a conceptual basic block) of name %q", name)
}

// getTerminator returns the terminator instruction of the named basic block (or
// primitive forming a conceptual basic block).
func getTerminator(bbs map[string]*basicBlock, prims map[string]*primitive, name string) (llvm.Value, error) {
	// Get terminator from basic block.
	if bb, ok := bbs[name]; ok {
		return bb.term, nil
	}

	// Get terminator from primitive.
	if prim, ok := prims[name]; ok {
		return prim.term, nil
	}

	return llvm.Value{}, errutil.Newf("unable to locate basic block (or primitive forming a conceptual basic block) of name %q", name)
}

// printMapping prints the mapping from sub node name to graph node name for an
// isomorphism of sub in graph.
func printMapping(graph *dot.Graph, sub *graphs.SubGraph, m map[string]string) {
	entry := m[sub.Entry()]
	var snames []string
	for sname := range m {
		snames = append(snames, sname)
	}
	sort.Strings(snames)
	fmt.Printf("Isomorphism of %q found at node %q:\n", sub.Name, entry)
	for _, sname := range snames {
		fmt.Printf("   %q=%q\n", sname, m[sname])
	}
}
