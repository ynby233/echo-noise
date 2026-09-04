package services

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestViewerReadAPIsDoNotAcceptLegacyAdministratorFlags(t *testing.T) {
	legacyReadFunctions := map[string]struct{}{
		"CanViewMessage":                       {},
		"ApplyMessageVisibilityScope":          {},
		"GetAllMessagesForViewer":              {},
		"GetMessageByIDForViewer":              {},
		"messagePageBaseQuery":                 {},
		"GetMessagesByPage":                    {},
		"LocateMessagePage":                    {},
		"IncrementLikeCount":                   {},
		"ToggleLike":                           {},
		"GetMessagesGroupByDate":               {},
		"GetMessagePage":                       {},
		"SearchMessages":                       {},
		"CanViewCommentInThread":               {},
		"EnsureVoceChatContactCachesForViewer": {},
		"countVisibleCommentStats":             {},
		"countReceivedInteractionStats":        {},
		"canViewComment":                       {},
		"buildVisibleUserNotifications":        {},
		"resolveImageViewer":                   {},
	}

	paths := []string{".", filepath.Join("..", "controllers")}
	files := token.NewFileSet()
	for _, path := range paths {
		packages, err := parser.ParseDir(files, path, func(info fs.FileInfo) bool {
			return filepath.Ext(info.Name()) == ".go"
		}, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, pkg := range packages {
			for _, file := range pkg.Files {
				for _, declaration := range file.Decls {
					function, ok := declaration.(*ast.FuncDecl)
					if !ok || function.Name == nil {
						continue
					}
					if _, tracked := legacyReadFunctions[function.Name.Name]; !tracked {
						continue
					}
					for _, field := range function.Type.Params.List {
						identifier, isBool := field.Type.(*ast.Ident)
						if !isBool || identifier.Name != "bool" {
							continue
						}
						for _, name := range field.Names {
							if name.Name == "isAdmin" || name.Name == "viewerIsAdmin" || name.Name == "_" {
								t.Errorf("%s still accepts legacy administrator flag %q", function.Name.Name, name.Name)
							}
						}
					}
				}
			}
		}
	}
}
