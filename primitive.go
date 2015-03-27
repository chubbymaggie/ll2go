// TODO: Verify that the if_return.dot primitive correctly maps
//    if A_cond {
//       return
//    }
// and not
//    if A_cond {
//       f()
//    }

package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"path/filepath"
	"sort"

	"decomp.org/x/graphs"
	"decomp.org/x/graphs/iso"
	"decomp.org/x/graphs/merge"
	"github.com/mewfork/dot"
	"github.com/mewkiz/pkg/errutil"
	"github.com/mewkiz/pkg/goutil"
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
func (prim *primitive) Name() string { return prim.name }

// Stmts returns the statements of the primitive, which conceptually represents
// a basic block.
func (prim *primitive) Stmts() []ast.Stmt { return prim.stmts }

// SetStmts sets the statements of the primitive, which conceptually represents
// a basic block.
func (prim *primitive) SetStmts(stmts []ast.Stmt) { prim.stmts = stmts }

// Term returns the terminator instruction of the primitive, which conceptually
// represents a basic block.
func (prim *primitive) Term() llvm.Value { return prim.term }

// restructure attempts to create a structured control flow for a function based
// on the provided control flow graph (which contains one node per basic block)
// and the function's basic blocks. It does so by repeatedly locating and
// merging structured subgraphs into single nodes until the entire graph is
// reduced into a single node or no structured subgraphs may be located.
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
		if !bb.Term().IsNil() {
			// TODO: Remove debug output.
			bb.Term().Dump()
			return nil, errutil.Newf("invalid terminator instruction of last basic block in function; expected nil since return statements are already handled")
		}
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
	case "if_else.dot":
		return createIfElsePrim(m, bbs, newName)
	case "if_return.dot":
		return createIfPrim(m, bbs, newName)
	case "list.dot":
		return createListPrim(m, bbs, newName)
	case "post_loop.dot":
		return createPostLoopPrim(m, bbs, newName)
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
	//
	//    A
	//    B
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
	//
	//    A
	//    if A_cond {
	//       B
	//    }
	//    C

	// Create if-statement.
	cond, _, _, err := getBrCond(bbCond.Term())
	if err != nil {
		return nil, errutil.Err(err)
	}
	ifStmt := &ast.IfStmt{
		Cond: cond,
		Body: &ast.BlockStmt{List: bbBody.Stmts()},
	}

	// Create primitive.
	stmts := append(bbCond.Stmts(), ifStmt)
	stmts = append(stmts, bbExit.Stmts()...)
	prim := &primitive{
		name:  newName,
		stmts: stmts,
		term:  bbExit.Term(),
	}
	return prim, nil
}

// createIfElsePrim creates an if-else primitive based on the identified
// subgraph, its node pair mapping and its basic blocks. The new control flow
// primitive conceptually represents a basic block with the given name.
//
// Contents of "if_else.dot":
//
//    digraph if_else {
//       A [label="entry"]
//       B
//       C
//       D [label="exit"]
//       A->B [label="true"]
//       A->C [label="false"]
//       B->D
//       C->D
//    }
func createIfElsePrim(m map[string]string, bbs map[string]BasicBlock, newName string) (*primitive, error) {
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
	nameD, ok := m["D"]
	if !ok {
		return nil, errutil.New(`unable to locate node pair for sub node "D"`)
	}
	bbCond, ok := bbs[nameA]
	if !ok {
		return nil, errutil.Newf("unable to locate basic block %q", nameA)
	}

	// The body nodes (B and C) of if-else primitives are indistinguishable at
	// the graph level. Verify their names against the terminator instruction of
	// the basic block and swap them if necessary.
	cond, targetTrue, targetFalse, err := getBrCond(bbCond.Term())
	if err != nil {
		return nil, errutil.Err(err)
	}
	if targetTrue != nameB && targetTrue != nameC {
		return nil, errutil.Newf("invalid target true branch; got %q, expected %q or %q", targetTrue, nameB, nameC)
	}
	if targetFalse != nameB && targetFalse != nameC {
		return nil, errutil.Newf("invalid target false branch; got %q, expected %q or %q", targetFalse, nameB, nameC)
	}
	fmt.Printf("B=%q, target true =%q\n", nameB, targetTrue)
	nameB = targetTrue
	fmt.Printf("C=%q, target false=%q\n", nameC, targetFalse)
	nameC = targetFalse

	bbBody1, ok := bbs[nameB]
	if !ok {
		return nil, errutil.Newf("unable to locate basic block %q", nameB)
	}
	bbBody2, ok := bbs[nameC]
	if !ok {
		return nil, errutil.Newf("unable to locate basic block %q", nameC)
	}
	bbExit, ok := bbs[nameD]
	if !ok {
		return nil, errutil.Newf("unable to locate basic block %q", nameD)
	}

	// Create and return new primitive.
	//
	//    A
	//    if A_cond {
	//       B
	//    } else {
	//       C
	//    }
	//    D

	// Create if-else statement.
	ifElseStmt := &ast.IfStmt{
		Cond: cond,
		Body: &ast.BlockStmt{List: bbBody1.Stmts()},
		Else: &ast.BlockStmt{List: bbBody2.Stmts()},
	}

	// Create primitive.
	stmts := append(bbCond.Stmts(), ifElseStmt)
	stmts = append(stmts, bbExit.Stmts()...)
	prim := &primitive{
		name:  newName,
		stmts: stmts,
		term:  bbExit.Term(),
	}
	return prim, nil
}

// createPreLoopPrim creates a pre-test loop primitive based on the identified
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

	// Locate and expand the condition.
	//
	//    // from:
	//    _2 := i < 10
	//    if _2 {
	//
	//    // to:
	//    if i < 10 {
	cond, _, _, err := getBrCond(bbCond.Term())
	if err != nil {
		return nil, errutil.Err(err)
	}
	cond, err = expand(bbCond, cond)
	if err != nil {
		return nil, errutil.Err(err)
	}

	if len(bbCond.Stmts()) != 0 {
		// Produce the following primitive instead of a regular for loop if A
		// contains statements.
		//
		//    for {
		//       A
		//       if !A_cond {
		//          break
		//       }
		//       B
		//    }
		//    C

		// Create if-statement.
		ifStmt := &ast.IfStmt{
			Cond: cond, // TODO: Negate condition?
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.BranchStmt{Tok: token.BREAK}}},
		}

		// Create for-loop.
		body := append(bbCond.Stmts(), ifStmt)
		body = append(body, bbBody.Stmts()...)
		forStmt := &ast.ForStmt{
			Body: &ast.BlockStmt{List: body},
		}

		// Create primitive.
		stmts := []ast.Stmt{forStmt}
		stmts = append(stmts, bbExit.Stmts()...)
		prim := &primitive{
			name:  newName,
			stmts: stmts,
			term:  bbExit.Term(),
		}
		return prim, nil
	}

	// Create and return new primitive.
	//
	//    for A_cond {
	//       B
	//    }
	//    C

	// Create for-loop.
	forStmt := &ast.ForStmt{
		Cond: cond,
		Body: &ast.BlockStmt{List: bbBody.Stmts()},
	}

	// Create primitive.
	stmts := []ast.Stmt{forStmt}
	stmts = append(stmts, bbExit.Stmts()...)
	prim := &primitive{
		name:  newName,
		stmts: stmts,
		term:  bbExit.Term(),
	}
	return prim, nil
}

// createPostLoopPrim creates a post-test loop primitive based on the identified
// subgraph, its node pair mapping and its basic blocks. The new control flow
// primitive conceptually represents a basic block with the given name.
//
// Contents of "post_loop.dot":
//
//    digraph post_loop {
//       A [label="entry"]
//       B [label="exit"]
//       A->A [label="true"]
//       A->B [label="false"]
//    }
func createPostLoopPrim(m map[string]string, bbs map[string]BasicBlock, newName string) (*primitive, error) {
	// Locate graph nodes.
	nameA, ok := m["A"]
	if !ok {
		return nil, errutil.New(`unable to locate node pair for sub node "A"`)
	}
	nameB, ok := m["B"]
	if !ok {
		return nil, errutil.New(`unable to locate node pair for sub node "B"`)
	}
	bbBody, ok := bbs[nameA]
	if !ok {
		return nil, errutil.Newf("unable to locate basic block %q", nameA)
	}
	bbExit, ok := bbs[nameB]
	if !ok {
		return nil, errutil.Newf("unable to locate basic block %q", nameB)
	}

	// Create and return new primitive.
	//
	//    for {
	//       A
	//       if !A_cond {
	//          break
	//       }
	//    }
	//    B

	// Create if-statement.
	cond, _, _, err := getBrCond(bbBody.Term())
	if err != nil {
		return nil, errutil.Err(err)
	}
	ifStmt := &ast.IfStmt{
		Cond: cond, // TODO: Negate condition?
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.BranchStmt{Tok: token.BREAK}}},
	}

	// Create for-loop.
	body := bbBody.Stmts()
	body = append(body, ifStmt)
	forStmt := &ast.ForStmt{
		Body: &ast.BlockStmt{List: body},
	}

	// Create primitive.
	stmts := []ast.Stmt{forStmt}
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
		"if.dot", "if_else.dot", "pre_loop.dot", "post_loop.dot", "if_return.dot",
		"list.dot",
	}
)

func init() {
	// Parse subgraphs representing common control flow primitives.
	subDir, err := goutil.SrcDir("decomp.org/x/graphs/testdata/primitives")
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
