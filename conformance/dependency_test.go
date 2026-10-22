package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestNoPanelPrivateOrProductionRuntimeDependency(t *testing.T) {
	metadata, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(string(metadata), "\r\n", "\n") != "module github.com/MalenkiySolovey/solovey-node\n\ngo 1.26.10\n" {
		t.Fatal("proof module gained dependency/toolchain drift; review explicit metadata change")
	}
	for _, name := range []string{"cmd", "daemon", "runtime", "database", "transport", "network", "installer", "frontend"} {
		if _, err := os.Stat("../" + name); !os.IsNotExist(err) {
			t.Fatalf("R6 gained production directory %s", name)
		}
	}
	err = filepath.WalkDir("..", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == ".tmp") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			return nil
		}
		source, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			return err
		}
		if source.Name.Name == "main" {
			t.Fatalf("R6 gained executable package: %s", name)
		}
		for _, imported := range source.Imports {
			dependency, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			own := dependency == "github.com/MalenkiySolovey/solovey-node/contracts/management/v1"
			if !own && strings.Contains(strings.Split(dependency, "/")[0], ".") {
				t.Fatalf("external/private coupling %s: %s", name, dependency)
			}
			if dependency == "net" || strings.HasPrefix(dependency, "net/") || dependency == "os/exec" ||
				dependency == "database/sql" || dependency == "syscall" || dependency == "plugin" {
				t.Fatalf("R6 gained transport/host execution dependency %s: %s", name, dependency)
			}
		}
		ast.Inspect(source, func(node ast.Node) bool {
			if declaration, ok := node.(*ast.FuncDecl); ok && declaration.Name.Name == "main" {
				t.Fatalf("R6 gained daemon entrypoint: %s", name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
