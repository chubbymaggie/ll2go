package main

import (
	"fmt"
	"go/ast"
	"log"
	"path/filepath"
	"sort"

	"github.com/mewfork/dot"
	"github.com/mewkiz/pkg/errutil"
	"github.com/mewkiz/pkg/goutil"
	"github.com/mewrev/graphs"
	"github.com/mewrev/graphs/iso"
	"github.com/mewrev/graphs/merge"
	"llvm.org/llvm/bindings/go/llvm"
)

// primitive represents a control flow primitive, such as a 2-way conditional, a
// pre-test loop or a single statement or a list of statements. Each primitive
// conceptually represents a basic block and may be treated as an instruction or
// a statement of other basic blocks.
type primitive struct {
	// The control flow primitive is conceptually a basic block, and as such
	// requires a basic block name.
	name string
	// Statements of the control flow primitive.
	stmts []ast.Stmt
	// Terminator instruction.
	term llvm.Value
}

// Name returns the name of the primitive, which conceptually represents a basic
// block.
func (bb *primitive) Name() string { return bb.name }

// Stmts returns the statements of the primitive, which conceptually represents
// a basic block.
func (bb *primitive) Stmts() []ast.Stmt { return bb.stmts }

// Term returns the terminator instruction of the primitive, which conceptually
// represents a basic block.
func (bb *primitive) Term() llvm.Value { return bb.term }

// restructure attempts to create a structured control flow for a function based
// on the provided graph (which contains one node per basic block) and the
// function's basic blocks. It does so by repeatedly locating and merging
// structured subgraphs into single nodes until the entire graph is reduced into
// a single node or no structured subgraphs may be located.
func restructure(graph *dot.Graph, bbs map[string]BasicBlock) (*ast.BlockStmt, error) {
	for len(bbs) > 1 {
		prim, err := locatePrim(graph, bbs)
		if err != nil {
			return nil, errutil.Err(err)
		}
		fmt.Println("located primitive:")
		printBB(prim)
		bbs[prim.Name()] = prim
	}
	fmt.Println("restructure: DONE :)")
	for _, bb := range bbs {
		fmt.Println("basic block:")
		printBB(bb)
		block := &ast.BlockStmt{
			List: bb.Stmts(),
		}
		return block, nil
	}
	return nil, errutil.New("unable to locate basic block")
}

// locatePrim locates a control flow primitive in the provided function's
// control flow graph and its basic blocks.
func locatePrim(graph *dot.Graph, bbs map[string]BasicBlock) (*primitive, error) {
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
		primBBs := make(map[string]BasicBlock)
		for _, gname := range m {
			bb, ok := bbs[gname]
			if !ok {
				return nil, errutil.Newf("unable to locate basic block %q", gname)
			}
			primBBs[gname] = bb
			delete(bbs, gname)
		}
		subName := subNames[i]
		return createPrim(subName, m, primBBs, newName)
	}

	return nil, errutil.New("unable to locate control flow primitive")
}

// createPrim creates a control flow primitive based on the identified subgraph
// and its node pair mapping and basic blocks. The new control flow primitive
// conceptually forms a new basic block with the specified name.
func createPrim(subName string, m map[string]string, bbs map[string]BasicBlock, newName string) (*primitive, error) {
	switch subName {
	case "if.dot":
		return createIfPrim(m, bbs, newName)
	//case "if_else.dot":
	//case "if_return.dot":
	case "list.dot":
		return createListPrim(m, bbs, newName)
	//case "post_loop.dot":
	case "pre_loop.dot":
		return createPreLoopPrim(m, bbs, newName)
	default:
		return nil, errutil.Newf("control flow primitive of subgraph %q not yet supported", subName)
	}
}

// createListPrim creates a list primitive containing a slice of Go statements
// based on the identified subgraph, its node pair mapping and its basic blocks.
// The new control flow primitive conceptually represents a basic block with the
// given name.
//
// Contents of "list.dot":
//
//    digraph list {
//       A [label="entry"]
//       B [label="exit"]
//       A->B
//    }
func createListPrim(m map[string]string, bbs map[string]BasicBlock, newName string) (*primitive, error) {
	// Locate graph nodes.
	nameA, ok := m["A"]
	if !ok {
		return nil, errutil.New(`unable to locate node pair for sub node "A"`)
	}
	nameB, ok := m["B"]
	if !ok {
		return nil, errutil.New(`unable to locate node pair for sub node "B"`)
	}
	bbEntry, ok := bbs[nameA]
	if !ok {
		return nil, errutil.Newf("unable to locate basic block %q", nameA)
	}
	bbExit, ok := bbs[nameB]
	if !ok {
		return nil, errutil.Newf("unable to locate basic block %q", nameB)
	}

	// Create and return new primitive.
	stmts := append(bbEntry.Stmts(), bbExit.Stmts()...)
	prim := &primitive{
		name:  newName,
		stmts: stmts,
		term:  bbExit.Term(),
	}
	return prim, nil
}

// createIfPrim creates an if-statement primitive based on the identified
// subgraph, its node pair mapping and its basic blocks. The new control flow
// primitive conceptually represents a basic block with the given name.
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
func createIfPrim(m map[string]string, bbs map[string]BasicBlock, newName string) (*primitive, error) {
	// Locate graph nodes.
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
	bbCond, ok := bbs[nameA]
	if !ok {
		return nil, errutil.Newf("unable to locate basic block %q", nameA)
	}
	bbBody, ok := bbs[nameB]
	if !ok {
		return nil, errutil.Newf("unable to locate basic block %q", nameB)
	}
	bbExit, ok := bbs[nameC]
	if !ok {
		return nil, errutil.Newf("unable to locate basic block %q", nameC)
	}

	// Create and return new primitive.
	cond, err := getBrCond(bbCond.Term())
	if err != nil {
		return nil, errutil.Err(err)
	}
	ifStmt := &ast.IfStmt{
		Cond: cond,
		Body: &ast.BlockStmt{List: bbBody.Stmts()},
	}
	stmts := append(bbCond.Stmts(), ifStmt)
	stmts = append(stmts, bbExit.Stmts()...)
	prim := &primitive{
		name:  newName,
		stmts: stmts,
		term:  bbExit.Term(),
	}
	return prim, nil
}

// createPreLoopPrim creates an pre-test loop primitive based on the identified
// subgraph, its node pair mapping and its basic blocks. The new control flow
// primitive conceptually represents a basic block with the given name.
//
// Contents of "pre_loop.dot":
//
//    digraph pre_loop {
//       A [label="entry"]
//       B
//       C [label="exit"]
//       A->B [label="true"]
//       B->A
//       A->C [label="false"]
//    }
func createPreLoopPrim(m map[string]string, bbs map[string]BasicBlock, newName string) (*primitive, error) {
	// Locate graph nodes.
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
	bbCond, ok := bbs[nameA]
	if !ok {
		return nil, errutil.Newf("unable to locate basic block %q", nameA)
	}
	bbBody, ok := bbs[nameB]
	if !ok {
		return nil, errutil.Newf("unable to locate basic block %q", nameB)
	}
	bbExit, ok := bbs[nameC]
	if !ok {
		return nil, errutil.Newf("unable to locate basic block %q", nameC)
	}

	// Create and return new primitive.
	cond, err := getBrCond(bbCond.Term())
	if err != nil {
		return nil, errutil.Err(err)
	}
	forStmt := &ast.ForStmt{
		Cond: cond,
		Body: &ast.BlockStmt{List: bbBody.Stmts()},
	}
	stmts := append(bbCond.Stmts(), forStmt)
	stmts = append(stmts, bbExit.Stmts()...)
	prim := &primitive{
		name:  newName,
		stmts: stmts,
		term:  bbExit.Term(),
	}
	return prim, nil
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
