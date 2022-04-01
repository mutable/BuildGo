package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
)

type attrsObj struct {
	Outputs map[string]string `json:"outputs"`
	System  string            `json:"system"`
	Params  struct {
		Name               string            `json:"name"`
		Mode               string            `json:"mode"`
		Path               string            `json:"path"`
		IsProgram          bool              `json:"isProgram"`
		IncludePath        []string          `json:"includePath"`
		InputFiles         map[string]string `json:"inputFiles"`
		InputAssemblyFiles map[string]string `json:"inputAssemblyFiles"`
		Archive            string            `json:"archive"`
		GoRoot             string            `json:"goRoot"`
		XDefs              map[string]string `json:"xDefs"`
	} `json:"params"`
}

func linkAllFiles(outDir string, inFiles map[string]string) {
	for name, path := range inFiles {
		if err := os.Symlink(path, filepath.Join(outDir, name)); err != nil {
			panic(err)
		}
	}
}

func runCmd(bin string, args ...string) {
	cmd := exec.Command(bin, args...)
	cmd.Env = append(cmd.Env, "GOROOT_FINAL=goroot")

	if msgs, err := cmd.CombinedOutput(); err != nil {
		os.Stderr.Write(msgs)
		panic(err)
	}
}

func main() {
	attrsBytes, err := ioutil.ReadFile(".attrs.json")
	if err != nil {
		panic(err)
	}

	var attrs attrsObj

	if err := json.Unmarshal(attrsBytes, &attrs); err != nil {
		panic(err)
	}

	bin := filepath.Join(attrs.Params.GoRoot, "bin/go")

	outputHash := filepath.Base(attrs.Outputs["out"])[:32]

	switch attrs.Params.Mode {
	case "compile":
		args := []string{"tool", "compile"}
		args = append(args, "-buildid", outputHash)
		for _, includePath := range attrs.Params.IncludePath {
			args = append(args, "-I", includePath)
		}

		outArchive := filepath.Join(attrs.Outputs["out"], attrs.Params.Path+".a")

		if !attrs.Params.IsProgram {
			args = append(args, "-p", attrs.Params.Path)
		}

		cwd, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		outPath := filepath.Join(cwd, attrs.Params.Path)

		if err := os.MkdirAll(outPath, 0700); err != nil {
			panic(err)
		}

		linkAllFiles(outPath, attrs.Params.InputFiles)
		linkAllFiles(outPath, attrs.Params.InputAssemblyFiles)

		args = append(args, "-pack", "-o", outArchive, "-trimpath", cwd)

		// if we have .s files, run go tool asm to generate the required files.
		// we need the symabis for mixing the assembly and Go code.
		if len(attrs.Params.InputAssemblyFiles) > 0 {
			sharedArgs := []string{
				"tool", "asm",
				"-trimpath", cwd,
				"-I", outPath,
				"-I", attrs.Params.GoRoot + "/share/go/pkg/include",
			}

			var files []string
			for name := range attrs.Params.InputAssemblyFiles {
				files = append(files, filepath.Join(outPath, name))
			}

			runCmd(bin, append(append(sharedArgs, "-gensymabis", "-o", "./symabis"), files...)...)
			runCmd(bin, append(append(sharedArgs, "-o", "./asm.o"), files...)...)

			args = append(args, "-symabis", "./symabis", "-asmhdr", filepath.Join(attrs.Outputs["out"], "go_asm.h"))
		}

		for file := range attrs.Params.InputFiles {
			args = append(args, filepath.Join(outPath, file))
		}

		if err := os.MkdirAll(filepath.Dir(outArchive), 0700); err != nil {
			panic(err)
		}

		runCmd(bin, args...)

		if len(attrs.Params.InputAssemblyFiles) > 0 {
			runCmd(bin, "tool", "pack", "r", outArchive, "./asm.o")
		}

	case "link":
		args := []string{"tool", "link", "-linkmode", "internal"}
		args = append(args, "-buildid", outputHash)
		for _, includePath := range attrs.Params.IncludePath {
			args = append(args, "-L", includePath)
		}

		for key, value := range attrs.Params.XDefs {
			args = append(args, "-X", fmt.Sprintf("%s=%s", key, value))
		}

		binPath := filepath.Join(attrs.Outputs["out"], "bin")
		if err := os.MkdirAll(binPath, 0700); err != nil {
			panic(err)
		}

		args = append(args, "-o", filepath.Join(binPath, attrs.Params.Name), attrs.Params.Archive)
		runCmd(bin, args...)

	default:
		panic("unknown mode")
	}
}
