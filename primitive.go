package main

import (
	"fmt"
	"go/ast"
	"log"
	"path/filepath"

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
	// TODO: Document additional node types.

	// node is one of the following types:
	//    []ast.Stmt
	node ast.Node
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
	case "list.dot":
		return createListPrim(m, bbs, prims, newName)
	//case "if.dot":
	//case "if_else.dot":
	//case "pre_loop.dot":
	//case "post_loop.dot":
	//case "if_return.dot":
	default:
		return nil, errutil.Newf("control flow primitive %q not yet supported", subName)
	}
}

// createListPrim creates a slice of Go statements based on the identified
// subgraph, its node pair mapping, and its basic block (and primitives forming
// conceptual basic blocks). The new control flow primitive conceptually forms a
// new basic block with the specified name.
func createListPrim(m map[string]string, bbs map[string]*basicBlock, prims map[string]*primitive, newName string) (prim *primitive, err error) {
	panic("not yet implemented")
}
