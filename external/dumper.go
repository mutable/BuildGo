package main

import (
	"encoding/json"
	"go/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type sourceFiles struct {
	Includes []string          `json:"includes,omitempty"`
	GoFiles  []string          `json:"goFiles,omitempty"`
	CGoFiles []string          `json:"cgoFiles,omitempty"`
	SFiles   []string          `json:"sFiles,omitempty"`
	HFiles   []string          `json:"hFiles,omitempty"`
	Flags    map[string]string `json:"flags,omitempty"`
}

type params struct {
	Path        string      `json:"path"`
	Dir         string      `json:"dir"`
	Sources     sourceFiles `json:"sources"`
	TestSources sourceFiles `json:"testSources"`
	IsProgram   bool        `json:"isProgram"`
}

func findPackages(root string) (map[string]bool, error) {
	outputs := make(map[string]bool)

	walkGoDirs := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() && filepath.Base(path) == "testdata" {
			return filepath.SkipDir
		}

		if info.Mode().IsRegular() && strings.HasSuffix(path, ".go") {
			outputs[filepath.Dir(path)] = true
		}

		return nil
	}

	if err := filepath.Walk(root, walkGoDirs); err != nil {
		return nil, err
	}

	return outputs, nil
}

func main() {
	rootDir := os.Args[1]
	rootPath := os.Args[2]

	pkgs, err := findPackages(rootDir)
	if err != nil {
		panic(err)
	}

	var outputs []params

	ctx := build.Default
	ctx.CgoEnabled = false
	ctx.BuildTags = []string{"containers_image_openpgp"}

	for pkg := range pkgs {
		pkgContents, err := ctx.ImportDir(pkg, build.IgnoreVendor)
		if _, ok := err.(*build.NoGoError); ok {
			continue
		} else if err != nil {
			panic(err)
		}

		sort.Strings(pkgContents.CgoCFLAGS)
		sort.Strings(pkgContents.CgoCPPFLAGS)
		sort.Strings(pkgContents.CgoLDFLAGS)

		relPath, err := filepath.Rel(rootDir, pkg)
		if err != nil {
			panic(err)
		}

		outputs = append(outputs, params{
			Path: filepath.Join(rootPath, relPath),
			Dir:  relPath,
			Sources: sourceFiles{
				Includes: pkgContents.Imports,
				GoFiles:  pkgContents.GoFiles,
				CGoFiles: pkgContents.CgoFiles,
				SFiles:   pkgContents.SFiles,
				HFiles:   pkgContents.HFiles,
				Flags: map[string]string{
					"CFLAGS":   strings.Join(pkgContents.CgoCFLAGS, " "),
					"CPPFLAGS": strings.Join(pkgContents.CgoCPPFLAGS, " "),
					"LDFLAGS":  strings.Join(pkgContents.CgoLDFLAGS, " "),
				},
			},
			TestSources: sourceFiles{
				Includes: pkgContents.TestImports,
				GoFiles:  pkgContents.TestGoFiles,
			},
			IsProgram: pkgContents.IsCommand(),
		})
	}

	data, err := json.Marshal(outputs)
	if err != nil {
		panic(err)
	}

	if err := ioutil.WriteFile(os.Getenv("out"), data, 0600); err != nil {
		panic(err)
	}
}
