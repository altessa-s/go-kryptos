// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package kryptos_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const module = "github.com/altessa-s/go-kryptos"

// The walker is derived from go-pythia and deliberately traverses nested modules.
var layers = map[string][]string{
	".":     {module + "/internal/clone", module + "/internal/zero"},
	"codec": {}, "internal/clone": {}, "internal/zero": {}, "internal/keystate": {},
	"internal/lru":    {"github.com/hashicorp/golang-lru/v2"},
	"codec/base64key": {}, "codec/jsonvalue": {},
	"manager":                {module, module + "/internal/clone", module + "/internal/keystate", module + "/internal/lru", "github.com/altessa-s/go-atlas-core/retry", "github.com/altessa-s/go-atlas-core/runtime/concurrency", "github.com/altessa-s/go-atlas-core/runtime/cycle", "github.com/altessa-s/go-atlas-core/runtime/singleflight"},
	"providers/memory":       {module},
	"providers/vault":        {module, module + "/internal/zero", module + "/codec/...", "github.com/hashicorp/vault/api"},
	"providers/gcp":          {module, module + "/internal/zero", module + "/codec/...", "cloud.google.com/go/secretmanager/...", "google.golang.org/..."},
	"providers/lockbox":      {module, module + "/internal/zero", module + "/codec/...", "github.com/yandex-cloud/go-genproto/...", "github.com/golang-jwt/jwt/v5", "google.golang.org/...", "github.com/altessa-s/go-atlas-net/httpx", "github.com/altessa-s/go-atlas-core/runtime/singleflight"},
	"kryptostest":            {module, "github.com/stretchr/testify/require", "github.com/altessa-s/go-atlas-core/runtime/leakcheck"},
	"integration/docker":     {},
	"providers/gcp/fake":     {"cloud.google.com/go/secretmanager/...", "google.golang.org/..."},
	"providers/lockbox/fake": {"github.com/yandex-cloud/go-genproto/...", "google.golang.org/..."},
	"devtools":               {}, "devtools/headers": {},
}

func matches(pattern, imp string) bool {
	if p, ok := strings.CutSuffix(pattern, "/..."); ok {
		return imp == p || strings.HasPrefix(imp, p+"/")
	}
	return pattern == imp
}
func TestArchitecture(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	modules := map[string]bool{}
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != "." && (strings.HasPrefix(d.Name(), ".") || d.Name() == "bin" || d.Name() == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		dir := filepath.ToSlash(filepath.Dir(p))
		if d.Name() == "go.mod" {
			modules[dir] = true
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}

		f, e := parser.ParseFile(token.NewFileSet(), p, nil, parser.ParseComments)
		require.NoError(t, e)
		allowed, ok := layers[dir]
		require.True(t, ok, "unlisted package: %s", dir)
		seen[dir] = true
		test := strings.HasSuffix(p, "_test.go")
		for _, spec := range f.Imports {
			imp, e := strconv.Unquote(spec.Path.Value)
			require.NoError(t, e)
			require.False(t, matches("github.com/altessa-s/go-atlas/...", imp), "monolith import in %s", p)
			if p == "devtools/tools.go" {
				continue
			}
			if strings.HasPrefix(imp, "github.com/altessa-s/") && !matches(module+"/...", imp) && !matches("github.com/altessa-s/go-atlas-core/...", imp) && !matches("github.com/altessa-s/go-atlas-net/...", imp) {
				require.True(t, dir == "integration/docker" && matches("github.com/altessa-s/go-nomos/...", imp), "sibling library in %s", p)
			}
			if test || p == "devtools/tools.go" {
				continue
			}
			first, _, _ := strings.Cut(imp, "/")
			if slices.Contains([]string{".", "codec", "internal/clone", "internal/zero"}, dir) && !strings.Contains(first, ".") {
				require.Contains(t, []string{"context", "errors", "fmt", "io", "iter", "log/slog", "reflect", "slices", "strings", "sync", "time", "strconv", "encoding/json"}, imp, "domain stdlib import in %s", p)
			}
			if !strings.Contains(first, ".") {
				if imp == "testing" {
					require.Equal(t, "kryptostest", dir)
				}
				require.NotEqual(t, "unsafe", imp)
				continue
			}
			require.True(t, slices.ContainsFunc(allowed, func(a string) bool { return matches(a, imp) }), "%s imports %s", p, imp)
		}
		if test {
			return nil
		}
		generated := strings.HasSuffix(p, "options_gen.go")
		aliases := map[string]string{}
		for _, imp := range f.Imports {
			name, _ := strconv.Unquote(imp.Path.Value)
			alias := filepath.Base(name)
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			aliases[alias] = name
		}
		var realClock []*ast.FuncDecl
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && dir == "." && fn.Recv != nil {
				for _, r := range fn.Recv.List {
					if id, ok := r.Type.(*ast.Ident); ok && id.Name == "RealClock" {
						realClock = append(realClock, fn)
					}
				}
			}
		}

		ast.Inspect(f, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.FuncDecl:
				require.NotEqual(t, "init", v.Name.Name, "%s", p)
				if strings.HasPrefix(v.Name.Name, "With") {
					require.True(t, generated, "hand-written option: %s", p)
				}
			case *ast.GenDecl:
				if v.Tok == token.VAR {
					for _, s := range v.Specs {
						for _, name := range s.(*ast.ValueSpec).Names {
							require.False(t, name.IsExported() && !strings.HasPrefix(name.Name, "Err"), "mutable exported variable: %s", p)
						}
					}
				}
			case *ast.TypeSpec:
				if v.Name.IsExported() && (strings.HasSuffix(v.Name.Name, "Config") || strings.HasSuffix(v.Name.Name, "Options")) {
					if st, ok := v.Type.(*ast.StructType); ok {
						for _, field := range st.Fields.List {
							for _, name := range field.Names {
								require.False(t, name.IsExported(), "public configuration: %s", p)
							}
						}
					}
				}
			case *ast.CompositeLit:
				if selector, ok := v.Type.(*ast.SelectorExpr); ok {
					if id, ok := selector.X.(*ast.Ident); ok {
						require.False(t, id.Name == "http" && selector.Sel.Name == "Client", "constructed HTTP client in %s", p)
					}
				}
			case *ast.SelectorExpr:
				if id, ok := v.X.(*ast.Ident); ok {
					require.False(t, id.Name == "log" || id.Name == "fmt" && strings.HasPrefix(v.Sel.Name, "Print"), "direct logging in %s", p)
					require.False(t, id.Name == "http" && v.Sel.Name == "DefaultClient", "%s", p)
					inClock := slices.ContainsFunc(realClock, func(fn *ast.FuncDecl) bool { return fn.Pos() <= v.Pos() && v.End() <= fn.End() })
					require.False(t, aliases[id.Name] == "time" && slices.Contains([]string{"Now", "Since", "Sleep", "After", "NewTimer", "NewTicker", "Tick"}, v.Sel.Name) && !inClock, "uninjected time: %s", p)
					require.False(t, aliases[id.Name] == "os" && slices.Contains([]string{"Getenv", "LookupEnv", "Environ", "ReadFile", "Open", "Create"}, v.Sel.Name) && dir != "kryptostest" && !strings.HasPrefix(dir, "devtools"), "configuration I/O: %s", p)
				}
			}
			return true
		})
		return nil
	})
	require.NoError(t, err)
	for dir := range seen {
		for _, name := range []string{"doc.go", "README.md"} {
			_, e := os.Stat(filepath.Join(dir, name))
			require.NoError(t, e, "%s", dir)
		}
	}
	for dir := range modules {
		require.Contains(t, []string{".", "providers/vault", "providers/gcp", "providers/lockbox", "integration/docker", "devtools"}, dir, "unlisted module")
	}
	for _, dir := range []string{".", "providers/vault", "providers/gcp", "providers/lockbox", "integration/docker", "devtools"} {
		require.True(t, modules[dir], "missing module %s", dir)
	}
}
func TestModulePurity(t *testing.T) {
	t.Parallel()
	for _, dir := range []string{".", "providers/vault", "providers/gcp", "providers/lockbox", "integration/docker", "devtools"} {
		cmd := exec.CommandContext(t.Context(), "go", "mod", "edit", "-json")
		cmd.Dir = dir
		b, err := cmd.Output()
		require.NoError(t, err)
		var mod struct {
			Require []struct {
				Path     string
				Indirect bool
			}
			Replace []struct {
				Old, New struct{ Path, Version string }
			}
		}
		require.NoError(t, json.Unmarshal(b, &mod))
		if dir == "." {
			for _, dep := range mod.Require {
				if !dep.Indirect {
					require.Contains(t, []string{"github.com/altessa-s/go-atlas-core", "github.com/hashicorp/golang-lru/v2", "github.com/stretchr/testify"}, dep.Path)
				}
			}
		}
		root, err := filepath.Abs(".")
		require.NoError(t, err)
		for _, replacement := range mod.Replace {
			require.Empty(t, replacement.New.Version)
			target, err := filepath.Abs(filepath.Join(dir, replacement.New.Path))
			require.NoError(t, err)
			expected := root
			// The test-only Docker module must exercise this checkout's unreleased Vault module.
			if dir == "integration/docker" && replacement.Old.Path == module+"/providers/vault" {
				expected = filepath.Join(root, "providers/vault")
			} else {
				require.Equal(t, module, replacement.Old.Path)
			}
			require.Equal(t, expected, target)
		}
	}
}
