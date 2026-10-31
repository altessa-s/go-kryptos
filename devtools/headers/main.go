// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bytes"
	"os"
)

// main adds the repository copyright to optgen outputs without changing generated declarations.
func main() {
	const header = "// Copyright 2021-2026 ALTESSA SOLUTIONS INC.\n// Use of this source code is governed by the MIT license that can be found in\n// the LICENSE file.\n\n"
	root, err := os.OpenRoot(".")
	if err != nil {
		panic(err)
	}
	defer root.Close()
	for _, name := range os.Args[1:] {
		if name == "--" {
			continue
		}
		b, e := root.ReadFile(name)
		if e != nil {
			panic(e)
		}
		if !bytes.HasPrefix(b, []byte("// Copyright")) {
			if e = root.WriteFile(name, append([]byte(header), b...), 0600); e != nil {
				panic(e)
			}
		}
	}
}
