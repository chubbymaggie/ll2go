## WIP

This project is a *work in progress*. The implementation is *incomplete* and subject to change. The documentation can be inaccurate.

# ll2go

[![GoDoc](https://godoc.org/github.com/mewrev/ll2go?status.svg)](https://godoc.org/github.com/mewrev/ll2go)

`ll2go` is a tool which decompiles LLVM IR assembly files to Go source code (e.g. *.ll -> *.go).

## Installation

```shell
go get github.com/mewrev/ll2go
```

## Usage

    ll2go [OPTION]... FILE...

    Flags:
      -f=false:    Force overwrite existing Go source code.
      -funcs="":   Comma separated list of functions to decompile (e.g. "foo,bar").
      -pkgname="": Package name.
      -q=false:    Suppress non-error messages.

## Examples

TODO

## Dependencies

* [llvm.org/llvm/bindings/go/llvm](https://godoc.org/llvm.org/llvm/bindings/go/llvm) with [unnamed.patch](unnamed.patch)
* `llvm-as` from [LLVM](http://llvm.org/)
* `dot` from [GraphViz](http://www.graphviz.org/)
* [ll2dot](https://github.com/mewrev/ll2dot)

## Public domain

The source code and any original content of this repository is hereby released into the [public domain].

[public domain]: https://creativecommons.org/publicdomain/zero/1.0/
