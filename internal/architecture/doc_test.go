package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// This file is the check behind docs/DEVELOPMENT.md, "Documenting the code": every package has one package comment
// and every exported name has a doc comment that starts with the name, which is the form go doc, pkgsite and the
// editors read. It parses the source (no compilation), so it sees both twins of a file with a build tag.

// wellKnownMethods implement an interface the reader already knows; a comment on them is not required.
var wellKnownMethods = map[string]bool{
	"Error": true, "String": true, "Write": true, "Read": true, "Close": true, "ServeHTTP": true,
	"MarshalJSON": true, "UnmarshalJSON": true, "Is": true, "Unwrap": true, "As": true,
	"Len": true, "Less": true, "Swap": true, "MarshalText": true, "UnmarshalText": true,
}

// docProblems returns what is wrong with the documentation of the Go packages under dir, one line each.
func docProblems(dir string) []string {
	var problems []string
	pkgs := map[string][]string{} // folder -> its non-test files
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "testdata" || d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".") && p != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
			pkgs[filepath.Dir(p)] = append(pkgs[filepath.Dir(p)], p)
		}
		return nil
	})
	folders := make([]string, 0, len(pkgs))
	for f := range pkgs {
		folders = append(folders, f)
	}
	sort.Strings(folders)
	for _, folder := range folders {
		problems = append(problems, docProblemsOfPackage(folder, pkgs[folder])...)
	}
	return problems
}

func docProblemsOfPackage(folder string, files []string) []string {
	var problems []string
	fset := token.NewFileSet()
	var withDoc []string
	pkgName := ""
	for _, file := range files {
		f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err != nil {
			problems = append(problems, file+": cannot be parsed: "+err.Error())
			continue
		}
		if generated(f) {
			continue
		}
		pkgName = f.Name.Name
		if f.Doc != nil {
			withDoc = append(withDoc, file)
			want := "Package " + f.Name.Name
			if f.Name.Name == "main" {
				want = "Command " + filepath.Base(folder)
			}
			if !strings.HasPrefix(f.Doc.Text(), want) {
				problems = append(problems, file+": the package comment must start with \""+want+"\"")
			}
		}
		problems = append(problems, exportedProblems(fset, file, f)...)
	}
	switch {
	case pkgName == "":
	case len(withDoc) == 0:
		problems = append(problems, folder+": the package has no package comment")
	case len(withDoc) > 1:
		problems = append(problems, folder+": the package has more than one package comment: "+strings.Join(withDoc, ", "))
	}
	return problems
}

func generated(f *ast.File) bool {
	for _, g := range f.Comments {
		for _, c := range g.List {
			if strings.HasPrefix(c.Text, "// Code generated ") && strings.HasSuffix(c.Text, " DO NOT EDIT.") {
				return true
			}
		}
	}
	return false
}

// startsWithName reports whether the doc starts with name (a type may take an article first).
func startsWithName(doc *ast.CommentGroup, name string, article bool) bool {
	if doc == nil {
		return false
	}
	text := strings.TrimSpace(doc.Text())
	if article {
		for _, a := range []string{"A ", "An ", "The "} {
			text = strings.TrimPrefix(text, a)
		}
	}
	if !strings.HasPrefix(text, name) {
		return false
	}
	rest := text[len(name):]
	// The name stands alone: a space, a line break or the end follows it (staticcheck's ST1020 asks for "Name ...").
	return rest == "" || rest[0] == ' ' || rest[0] == '\n'
}

func exportedProblems(fset *token.FileSet, file string, f *ast.File) []string {
	var problems []string
	at := func(pos token.Pos, name, what string) {
		problems = append(problems, fset.Position(pos).String()+": "+what+" "+name)
	}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if !d.Name.IsExported() {
				continue
			}
			if d.Recv != nil {
				recv := receiverName(d.Recv)
				if !ast.IsExported(recv) || wellKnownMethods[d.Name.Name] {
					continue
				}
			}
			if d.Doc == nil {
				at(d.Pos(), d.Name.Name, "has no doc comment:")
			} else if !startsWithName(d.Doc, d.Name.Name, false) {
				at(d.Pos(), d.Name.Name, "doc comment must start with the name:")
			}
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if !s.Name.IsExported() {
						continue
					}
					doc := s.Doc
					if doc == nil && len(d.Specs) == 1 {
						doc = d.Doc
					}
					if doc == nil {
						at(s.Pos(), s.Name.Name, "has no doc comment:")
					} else if !startsWithName(doc, s.Name.Name, true) {
						at(s.Pos(), s.Name.Name, "doc comment must start with the name:")
					}
				case *ast.ValueSpec:
					for _, n := range s.Names {
						if !n.IsExported() {
							continue
						}
						doc := s.Doc
						grouped := d.Lparen.IsValid()
						if doc == nil && !grouped {
							doc = d.Doc
						}
						switch {
						case doc != nil && !grouped && !startsWithName(doc, n.Name, false):
							at(n.Pos(), n.Name, "doc comment must start with the name:")
						case doc != nil && grouped && s.Doc != nil && !startsWithName(doc, n.Name, false):
							at(n.Pos(), n.Name, "doc comment must start with the name:")
						case doc == nil && !(grouped && d.Doc != nil):
							at(n.Pos(), n.Name, "has no doc comment:")
						}
					}
				}
			}
		}
	}
	return problems
}

func receiverName(r *ast.FieldList) string {
	if r == nil || len(r.List) == 0 {
		return ""
	}
	t := r.List[0].Type
	for {
		switch x := t.(type) {
		case *ast.StarExpr:
			t = x.X
		case *ast.IndexExpr:
			t = x.X
		case *ast.IndexListExpr:
			t = x.X
		case *ast.Ident:
			return x.Name
		default:
			return ""
		}
	}
}

func TestDocChecker_AcceptsADocumentedPackage(t *testing.T) {
	if p := docProblems(filepath.Join("testdata", "doc", "good")); len(p) != 0 {
		t.Fatalf("the documented package must pass: %v", p)
	}
}

func TestDocChecker_NamesWhatIsWrong(t *testing.T) {
	got := strings.Join(docProblems(filepath.Join("testdata", "doc", "bad")), "\n")
	for _, want := range []string{
		"more than one package comment",
		"NoComment",
		"Model",
		"doc comment must start with the name",
		"Undocumented",
		"Exposed",
		"Other",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the checker did not report %q; it said:\n%s", want, got)
		}
	}
}

// TestEveryPackageAndExportedNameIsDocumented: the rule of docs/DEVELOPMENT.md, "Documenting the code".
func TestEveryPackageAndExportedNameIsDocumented(t *testing.T) {
	var all []string
	for _, root := range []string{filepath.Join("..", "..", "cmd"), ".."} {
		all = append(all, docProblems(root)...)
	}
	if len(all) > 0 {
		t.Errorf("%d documentation problems (see docs/DEVELOPMENT.md, \"Documenting the code\"):\n%s", len(all), strings.Join(all, "\n"))
	}
}
