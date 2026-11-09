package conformance

import (
	"encoding/json"
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

const publicContractImport = "github.com/MalenkiySolovey/solovey-contracts/management/v1"

func forbiddenProofImport(dependency string) bool {
	if dependency != publicContractImport && strings.Contains(strings.Split(dependency, "/")[0], ".") {
		return true
	}
	return dependency == "net" || strings.HasPrefix(dependency, "net/") || dependency == "os/exec" ||
		dependency == "database/sql" || dependency == "syscall" || dependency == "plugin"
}

func TestDependencyRatchetRejectsPrivateAndExecutionImports(t *testing.T) {
	for _, dependency := range []string{
		"github.com/MalenkiySolovey/solovey-ui/internal/secrets",
		"github.com/MalenkiySolovey/solovey-ui/service",
		"github.com/MalenkiySolovey/solovey-ui/database",
		"github.com/MalenkiySolovey/solovey-contracts/internal/private",
		"example.invalid/other-module", "net", "net/http", "os/exec", "database/sql", "syscall", "plugin",
	} {
		t.Run(dependency, func(t *testing.T) {
			if !forbiddenProofImport(dependency) {
				t.Fatalf("private/production dependency accepted: %s", dependency)
			}
		})
	}
	for _, dependency := range []string{publicContractImport, "encoding/json", "testing"} {
		if forbiddenProofImport(dependency) {
			t.Fatalf("public/stdlib proof dependency denied: %s", dependency)
		}
	}
}

func TestNoPanelPrivateOrProductionRuntimeDependency(t *testing.T) {
	metadata, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	pinJSON, err := os.ReadFile("../contracts/PIN.json")
	if err != nil {
		t.Fatal(err)
	}
	var pin struct{ Module, Version string }
	if err := json.Unmarshal(pinJSON, &pin); err != nil || pin.Module != "github.com/MalenkiySolovey/solovey-contracts" || pin.Version == "" {
		t.Fatal("missing accepted Contracts pin")
	}
	expected := "module github.com/MalenkiySolovey/solovey-node\n\ngo 1.26.10\n\nrequire " + pin.Module + " " + pin.Version + "\n"
	if strings.ReplaceAll(string(metadata), "\r\n", "\n") != expected {
		t.Fatal("proof module gained unapproved dependency/toolchain/replace drift")
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
			if forbiddenProofImport(dependency) {
				t.Fatalf("external/private/production coupling %s: %s", name, dependency)
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
