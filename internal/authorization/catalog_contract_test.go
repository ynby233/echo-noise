package authorization

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestCatalogDefinitionsAreUniqueStableAndComplete(t *testing.T) {
	catalogDefinitions := Catalog()
	byCapability := make(map[Capability]Definition, len(catalogDefinitions))
	for _, definition := range catalogDefinitions {
		if definition.Capability == "" || definition.Module == "" || definition.Label == "" {
			t.Fatalf("catalog definitions must have non-empty capability, module, and label: %#v", definition)
		}
		if !strings.Contains(string(definition.Capability), ".") {
			t.Fatalf("capability must have a stable module.action shape: %q", definition.Capability)
		}
		if _, exists := byCapability[definition.Capability]; exists {
			t.Fatalf("catalog contains duplicate capability %q", definition.Capability)
		}
		byCapability[definition.Capability] = definition
	}

	declared := declaredCapabilityConstants(t)
	if len(declared) != len(catalogDefinitions) {
		t.Fatalf("catalog capability count=%d, declared capability constant count=%d", len(catalogDefinitions), len(declared))
	}
	for name, capability := range declared {
		if _, exists := byCapability[capability]; !exists {
			t.Errorf("Capability%s=%q is missing from Catalog", name, capability)
		}
	}
	for capability := range byCapability {
		found := false
		for _, declaredCapability := range declared {
			if declaredCapability == capability {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Catalog contains undeclared capability %q", capability)
		}
	}
}

func declaredCapabilityConstants(t *testing.T) map[string]Capability {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate catalog contract test")
	}
	sourcePath := filepath.Join(filepath.Dir(currentFile), "authorization.go")
	file, err := parser.ParseFile(token.NewFileSet(), sourcePath, nil, 0)
	if err != nil {
		t.Fatalf("parse capability source: %v", err)
	}

	declared := map[string]Capability{}
	for _, declaration := range file.Decls {
		gen, ok := declaration.(*ast.GenDecl)
		if !ok || gen.Tok.String() != "const" {
			continue
		}
		for _, spec := range gen.Specs {
			values, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, name := range values.Names {
				if !strings.HasPrefix(name.Name, "Capability") || index >= len(values.Values) {
					continue
				}
				literal, ok := values.Values[index].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					t.Fatalf("Capability%s must use a string literal", strings.TrimPrefix(name.Name, "Capability"))
				}
				value, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatalf("decode Capability%s: %v", strings.TrimPrefix(name.Name, "Capability"), err)
				}
				declared[strings.TrimPrefix(name.Name, "Capability")] = Capability(value)
			}
		}
	}
	if len(declared) == 0 {
		t.Fatal("no Capability constants found")
	}
	return declared
}
