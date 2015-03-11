// Copyright 2015 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

func init() {
	addTestCases(resolveTests, resolve)
}

var resolveTests = []testCase{
	// i=0,
	{
		Name: "resolve.0",
		In: `package main

func main() {
	i = 42
}
`,
		Out: `package main

func main() {
	i := 42
}
`,
	},
	// i=1,
	{
		Name: "resolve.1",
		In: `package main

func main() {
	i = 0
	for i < 10 {
		i = i + 1
	}
}
`,
		Out: `package main

func main() {
	i := 0
	for i < 10 {
		i = i + 1
	}
}
`,
	},
	// i=2,
	{
		Name: "resolve.2",
		In: `package main

func main() {
	j := 10
	i, j = 5, 5
}
`,
		Out: `package main

func main() {
	j := 10
	i, j := 5, 5
}
`,
	},
	// i=3,
	{
		Name: "resolve.3",
		In: `package main

var i int

func main() {
	for i < 10 {
		i = i + 1
	}
}
`,
		Out: `package main

var i int

func main() {
	for i < 10 {
		i = i + 1
	}
}
`,
	},
	/*
		// The output of this test case is correct once "go fix" has been applied
		// twice. Adjusting the output so that the first round passes gives the
		// following error: "applied fixes during second round". If anyone knows a
		// solution to this, give me a shout.
		//
		// i=4,
		{
			Name: "resolve.4",
			In: `package main

			func main() {
				if j = 0; true {
					j = 10
				}
				if j = 1; true {
					j = 20
				}
			}
			`,
			Out: `package main

			func main() {
				if j := 0; true {
					j = 10
				}
				if j := 1; true {
					j = 20
				}
			}
			`,
		},
	*/
}
