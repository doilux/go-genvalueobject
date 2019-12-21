package main

import (
	"flag"
	"fmt"
	"go-genvalueobject/genvalueobject"
	"go/ast"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const generateFileNameSuffix = "_value_gen.go"

func main() {
	inputStructNames := flag.String("types", "", "target struct names(comma separated ex)Employee,Department... )")
	flag.Parse()

	fmt.Println(flag.Args())

	if err := Main(flag.Args(), strings.Split(*inputStructNames, ",")); err != nil {
		log.Print(err)
		fmt.Printf(`
Useage: %s -types=[targetStruct,...] [targetDir]
`, os.Args[0])
	}
}

//func main() {
//	Main(nil, nil)
//}

func Main(args []string, structs []string) error {
	targetDir := "."
	if len(args) > 0 {
		targetDir = args[0]
	}



	//targetDir := "./_exsample"
	//structs = []string{"Employee"}

	fmt.Println(targetDir)
	fmt.Println(structs)

	if err := genvalueobject.Run(
		targetDir,
		genvalueobject.TargetStructs(structs),

		// {{パッケージ名}}_value_gen.goというファイルを生成する
		func(pkg *ast.Package) io.Writer {
			dstFileName := fmt.Sprintf("%s" + generateFileNameSuffix, pkg.Name)
			dstFilePath := filepath.Join(filepath.FromSlash(targetDir), dstFileName)
			f, err := os.Create(dstFilePath)
			if err != nil {
				panic(err)
			}
			return f
		},

		// .*_test.goと、本ツールで生成したファイルは処理対象外
		genvalueobject.WithFileFilter(
			func(finfo os.FileInfo) bool {
				return !strings.HasSuffix(finfo.Name(), "_test.go") &&
					!strings.HasSuffix(finfo.Name(), generateFileNameSuffix)
			},
		),
	); err != nil {
		return err
	}
	return nil
}
