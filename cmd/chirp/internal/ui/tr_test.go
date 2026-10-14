// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/barahn/chirp/client"
)

// speak chooses locale's words for the rest of the test, and English after.
func speak(t *testing.T, locale string) {
	t.Helper()
	if err := UseLanguage(locale); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { words = nil })
}

// keys reads the package's code for every text it translates: the first
// argument of each call to tr or trf, a string or a constant naming one. A
// text built at run time could not be checked here, so it fails the test.
func keys(t *testing.T) map[string]bool {
	t.Helper()
	fset := gotoken.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, e := range entries {
		// tr.go defines tr and trf, and passes them what it is given.
		if n := e.Name(); strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") && n != "tr.go" {
			f, err := parser.ParseFile(fset, n, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, f)
		}
	}

	consts := map[string]ast.Expr{}
	for _, f := range files {
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != gotoken.CONST {
				continue
			}
			for _, s := range g.Specs {
				v := s.(*ast.ValueSpec)
				for i, n := range v.Names {
					if i < len(v.Values) {
						consts[n.Name] = v.Values[i]
					}
				}
			}
		}
	}
	var text func(e ast.Expr) (string, bool)
	text = func(e ast.Expr) (string, bool) {
		switch e := e.(type) {
		case *ast.BasicLit:
			if e.Kind == gotoken.STRING {
				s, err := strconv.Unquote(e.Value)
				return s, err == nil
			}
		case *ast.Ident:
			if v, ok := consts[e.Name]; ok {
				return text(v)
			}
		}
		return "", false
	}

	found := map[string]bool{}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			if fn, ok := call.Fun.(*ast.Ident); !ok || (fn.Name != "tr" && fn.Name != "trf") {
				return true
			}
			s, ok := text(call.Args[0])
			if !ok {
				t.Errorf("%s: translates a text built at run time; pass a string or a constant", fset.Position(call.Pos()))
				return true
			}
			found[s] = true
			return true
		})
	}
	return found
}

var verb = regexp.MustCompile(`%[-+# 0]*[0-9]*(?:\.[0-9]+)?[a-zA-Z%]`)

// Every text the window translates has a translation in every language, with
// the same formatting verbs in the same order, and no translation is left
// over from a text the code no longer says. A changed English text is a new
// key, so its old translation shows up here as left over.
func TestEveryWordIsTranslated(t *testing.T) {
	want := keys(t)
	if len(want) < 50 {
		t.Fatalf("found only %d texts to translate; is the parser reading the package?", len(want))
	}
	for lang, file := range languages {
		got, err := loadWords(file)
		if err != nil {
			t.Fatal(err)
		}
		var missing, stale []string
		for k := range want {
			if got[k] == "" {
				missing = append(missing, k)
			} else if a, b := verb.FindAllString(k, -1), verb.FindAllString(got[k], -1); !slices.Equal(a, b) {
				t.Errorf("%s: %q has verbs %v, its translation %v", lang, k, a, b)
			}
		}
		for k := range got {
			if !want[k] {
				stale = append(stale, k)
			}
		}
		sort.Strings(missing)
		sort.Strings(stale)
		for _, k := range missing {
			t.Errorf("%s: no translation for %q", lang, k)
		}
		for _, k := range stale {
			t.Errorf("%s: translation of %q, which the code no longer says", lang, k)
		}
	}
}

func TestTheLanguageFollowsTheLocale(t *testing.T) {
	for _, tc := range []struct{ locale, ready string }{
		{"pt-BR", "Pronto"},
		{"pt_BR", "Pronto"},
		{"pt-PT", "Pronto"},
		{"en-US", "Ready"},
		{"fr", "Ready"},
		{"", "Ready"},
	} {
		speak(t, tc.locale)
		if got := statusText(client.OwnerReady); got != tc.ready {
			t.Errorf("in %q, Ready is %q", tc.locale, got)
		}
	}
}

func TestThePromptSpeaksPortuguese(t *testing.T) {
	speak(t, "pt-BR")
	for got, want := range map[string]string{
		tr(promptTitle):      "Alguém quer controlar este computador",
		promptName("Ana"):    "Diz se chamar “Ana”.",
		tr(promptNoCheck):    "O Chirp não verifica nomes.",
		promptCountdown(100): "Recusa sozinho em 1:40",
		allowLabel(3):        "Permitir (3)",
		tr(promptDecline):    "Recusar",
	} {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
