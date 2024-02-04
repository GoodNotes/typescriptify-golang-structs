package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

type arrayImports []string

func (i *arrayImports) String() string {
	return "// custom imports:\n\n" + strings.Join(*i, "\n")
}

func (i *arrayImports) Set(value string) error {
	*i = append(*i, value)
	return nil
}

const TEMPLATE = `package main

import (
	"fmt"
{{- if .AllOptional }}
	"reflect"
{{- end }}

	m "{{ .ModelsPackage }}"
	"github.com/GoodNotes/typescriptify-golang-structs/typescriptify"
)

func main() {
{{ if .AllOptional }}
{{ range .Structs }}	{{ . }}Optional := typescriptify.TagAll(reflect.TypeOf(m.{{ . }}{}), []string{"omitempty"})
{{ end }}
{{ end }}
	t := typescriptify.New()
	t.CreateInterface = {{ .Interface }}
	t.ReadOnlyFields = {{ .Readonly }}
{{ range $key, $value := .InitParams }}	t.{{ $key }}={{ $value }}
{{ end }}
{{ if .AllOptional }}
{{ range .Structs }}	t.AddTypeWithName({{ . }}Optional, "{{ . }}")
{{ end }}
{{ else }}
{{ range .Structs }}	t.Add(m.{{ . }}{})
{{ end }}
{{ end }}
{{ range .CustomImports }}	t.AddImport("{{ . }}")
{{ end }}
	err := t.ConvertToFile("{{ .TargetFile }}")
	if err != nil {
		panic(err.Error())
	}
	fmt.Println("OK")
}`

type Params struct {
	ModelsPackage string
	TargetFile    string
	Structs       []string
	InitParams    map[string]interface{}
	CustomImports arrayImports
	Interface     bool
	Readonly      bool
	AllOptional   bool
	Verbose       bool
}

func main() {
	var p Params
	var backupDir string
	flag.StringVar(&p.ModelsPackage, "package", "", "Path of the package with models")
	flag.StringVar(&p.TargetFile, "target", "", "Target typescript file")
	flag.StringVar(&backupDir, "backup", "", "Directory where backup files are saved")
	flag.BoolVar(&p.Interface, "interface", false, "Create interfaces (not classes)")
	flag.BoolVar(&p.Readonly, "readonly", false, "Create interfaces with readonly fields")
	flag.BoolVar(&p.AllOptional, "all-optional", false, "Create interfaces with all fields optional")
	flag.Var(&p.CustomImports, "import", "Typescript import for your custom type, repeat this option for each import needed")
	flag.BoolVar(&p.Verbose, "verbose", false, "Verbose logs")
	flag.Parse()

	structs := []string{}
	for _, structOrGoFile := range flag.Args() {
		if strings.HasSuffix(structOrGoFile, ".go") {
			fmt.Println("Parsing:", structOrGoFile)
			fileStructs, err := GetGolangFileStructs(structOrGoFile)
			if err != nil {
				panic(fmt.Sprintf("Error loading/parsing golang file %s: %s", structOrGoFile, err.Error()))
			}
			structs = append(structs, fileStructs...)
		} else {
			structs = append(structs, structOrGoFile)
		}
	}

	if len(p.ModelsPackage) == 0 {
		fmt.Fprintln(os.Stderr, "No package given")
		os.Exit(1)
	}
	if len(p.TargetFile) == 0 {
		fmt.Fprintln(os.Stderr, "No target file")
		os.Exit(1)
	}

	t := template.Must(template.New("").Parse(TEMPLATE))

	d, err := os.MkdirTemp("", "tscriptify")
	handleErr(err)
	defer os.RemoveAll(d)

	f, err := os.CreateTemp(d, "main*.go")
	handleErr(err)
	defer f.Close()

	structsArr := make([]string, 0)
	for _, str := range structs {
		str = strings.TrimSpace(str)
		if len(str) > 0 {
			structsArr = append(structsArr, str)
		}
	}

	p.Structs = structsArr
	p.InitParams = map[string]interface{}{
		"BackupDir": fmt.Sprintf(`"%s"`, backupDir),
	}
	err = t.Execute(f, p)
	handleErr(err)

	if p.Verbose {
		byts, err := os.ReadFile(f.Name())
		handleErr(err)
		fmt.Printf("\nCompiling generated code (%s):\n%s\n----------------------------------------------------------------------------------------------------\n", f.Name(), string(byts))
	}

	var cmd *exec.Cmd
	cmdInit := exec.Command("go", "mod", "init", "tmp")
	fmt.Println(d + ": " + strings.Join(cmdInit.Args, " "))
	cmdInit.Dir = d
	initOutput, err := cmdInit.CombinedOutput()
	if err != nil {
		fmt.Println(string(initOutput))
		handleErr(err)
	}
	fmt.Println(string(initOutput))
	cmdGet := exec.Command("go", "get", "-v")
	cmdGet.Env = append(os.Environ(), "GO111MODULE=on")
	fmt.Println(d + ": " + strings.Join(cmdGet.Args, " "))
	cmdGet.Dir = d
	getOutput, err := cmdGet.CombinedOutput()
	if err != nil {
		fmt.Println(string(getOutput))
		handleErr(err)
	}
	fmt.Println(string(getOutput))
	cmd = exec.Command("go", "run", ".")
	cmd.Dir = d
	fmt.Println(d + ": " + strings.Join(cmd.Args, " "))

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println(string(output))
		handleErr(err)
	}
	fmt.Println(string(output))
	err = os.Rename(filepath.Join(d, p.TargetFile), p.TargetFile)
	handleErr(err)
}

func GetGolangFileStructs(filename string) ([]string, error) {
	fset := token.NewFileSet() // positions are relative to fset

	f, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return nil, err
	}

	v := &AVisitor{}
	ast.Walk(v, f)

	return v.structs, nil
}

type AVisitor struct {
	structNameCandidate string
	structs             []string
}

func (v *AVisitor) Visit(node ast.Node) ast.Visitor {
	if node != nil {
		switch t := node.(type) {
		case *ast.Ident:
			v.structNameCandidate = t.Name
		case *ast.StructType:
			if len(v.structNameCandidate) > 0 {
				v.structs = append(v.structs, v.structNameCandidate)
				v.structNameCandidate = ""
			}
		default:
			v.structNameCandidate = ""
		}
	}
	return v
}

func handleErr(err error) {
	if err != nil {
		panic(err.Error())
	}
}
