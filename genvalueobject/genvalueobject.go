/*
genvalueobject はフィールドのGetterと全てのフィールドを引数に受け取るコンストラクタを生成するジェネレータ
（Javaのライブラリのlombokの@Valueとほぼ同等の機能を目指している　参考：https://projectlombok.org/features/Value
with `go generate` command

```go
    //go:generate go-genvalueobject -Type=Employees,Departments...
```
*/
package genvalueobject

import (
	"bytes"
	"fmt"
	"github.com/doilux/go-strcase"
	"go/ast"
	"go/format"
	"io"
	"os"
	"strings"
	"text/template"

	"github.com/doilux/go-genutil/genutil"
)

type Option func(o *option)

type option struct {
	fileFilter    func(finfo os.FileInfo) bool
	generatorName string
}

// WithFileFilter は走査対象ファイルのフィルターを持つオプションを返す
func WithFileFilter(fileFilter func(finfo os.FileInfo) bool) Option {
	return func(o *option) {
		o.fileFilter = fileFilter
	}
}

// TargetStructs は生成対象のStruct
type TargetStructs []string

// contains はrcvにtypeNameが含まれるときにtrueを返す
func (rcv TargetStructs) contains(typeName string) bool {
	for _, v := range rcv {
		if v == typeName {
			return true
		}
	}
	return false
}

// Run はコードを生成する
func Run(targetDir string, targetTypes TargetStructs, newWriter func(pkg *ast.Package) io.Writer, opts ...Option) error {
	option := option{
		generatorName: "go-genvalueobject",
	}
	for _, opt := range opts {
		opt(&option)
	}

	walkers, err := genutil.DirToAstWalker(targetDir, option.fileFilter)
	if err != nil {
		return err
	}

	for _, walker := range walkers {
		body := new(bytes.Buffer)
		importPackages := make(map[string]string, 10)
		for _, spec := range walker.AllStructSpecs() {

			// 型名がtargetTypesに含まれるときにコードを生成する
			if targetTypes.contains(spec.Name.Name) {
				structType := spec.Type.(*ast.StructType)

				// コンストラクタと全フィールドのGetterメソッドをつくるため、
				// Structを解析し、フィールド名と型名を抽出する
				fts := make([]*fieldNameAndType, 0, 10)
				for _, field := range structType.Fields.List {
					typePrinter, err := walker.ToTypePrinter(field.Type)
					if err != nil {
						return err
					}
					fieldName := genutil.ParseFieldName(field)
					typeName := typePrinter.Print(walker.PkgPath)

					fts = append(fts, &fieldNameAndType{
						name:     fieldName,
						typeName: typeName,
					})
					for n, pkg := range typePrinter.ImportPkgMap(walker.PkgPath) {
						importPackages[n] = pkg
					}
				}

				// コンストラクタの生成
				if err := constructorTmpl.Execute(body, constructorTmplParams{
					StructName: spec.Name.Name,
					Argments:   fieldNameAndTypes(fts).getArgmentsStr(),
					Fields:  fieldNameAndTypes(fts).getFieldSetStr(),
				}); err != nil {
					panic(err)
				}

				// 各フィールドのGetterの生成
				for _, v := range fts {
					if err := getterTmpl.Execute(body, getterTmplParam{
						StructName: spec.Name.Name,
						MethodName: v.getGetterMethodName(),
						FieldType:  v.typeName,
						ZeroValue:  v.getTypeZeroValueStr(spec.Name.Name),
						FieldName:  v.name,
					}); err != nil {
						panic(err)
					}
				}
			}
		}


		if body.Len() == 0 {
			continue
		}

		out := new(bytes.Buffer)

		err = template.Must(template.New("out").Parse(`
			// Code generated by {{ .GeneratorName }}; DO NOT EDIT.
		
			package {{ .PackageName }}
		
			{{ .ImportPackages }}
		
			{{ .Body }}
		`)).Execute(out, map[string]string{
			"GeneratorName":  option.generatorName,
			"PackageName":    walker.Pkg.Name,
			"ImportPackages": genutil.GoFmtImports(importPackages),
			"Body":           body.String(),
		})
		if err != nil {
			return err
		}

		fmt.Println(string(out.Bytes()))

		str, err := format.Source(out.Bytes())
		if err != nil {
			return err
		}
		writer := newWriter(walker.Pkg)
		if closer, ok := writer.(io.Closer); ok {
			defer closer.Close()
		}
		if _, err := writer.Write(str); err != nil {
			return err
		}
	}

	return nil
}

// fieldNameAndTypes はfieldNameAndTypeのスライス
type fieldNameAndTypes []*fieldNameAndType

// fieldNameAndType はフィールド名と型名
type fieldNameAndType struct {
	name     string
	typeName string
}

// getArgmentsStr はコンストラクタの引数部分の文字列を生成する
// ex) id int, name string,...
func (rcv fieldNameAndTypes) getArgmentsStr() string {
	var result string
	for i, v := range rcv {
		result = result + v.name + " " + v.typeName
		if i != len(rcv)-1 {
			result = result + ","
		}
	}
	return result
}

// getFieldSetStr はコンストラクタでフィールドに値をセットする部分の文字列を生成する
// ex)
//  id:   id,
//  name: name,
//  ...
func (rcv fieldNameAndTypes) getFieldSetStr() string {
	result := "\n"
	for _, v := range rcv {
		result = result + v.name + ":" + v.name + ","
	}
	result = result + "\n"
	return result
}

// getTypeZeroValueStr はフィールドのZero値を返す
// プリミティブ型の場合は型に応じた値、 ポインタ型、スライス、マップはnil
// 上記以外はインスタンスをZeroValueで埋めてから取得し返却する。
func (rcv fieldNameAndType) getTypeZeroValueStr(structTypeName string) string {
	// ポインタ型の場合
	if strings.HasPrefix(rcv.typeName, "*") {
		return "nil"
	}

	// スライス
	if strings.HasPrefix(rcv.typeName, "[]") {
		return "nil"
	}

	// マップ
	if strings.HasPrefix(rcv.typeName, "map") {
		return "nil"
	}

	// プリミティブ
	switch rcv.typeName {
	case "string":
		return "\"\""
	case "rune", "byte", "uintptr":
		return "0"
	case	"bool":
		return "false"
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64":
			return "0"
	case "float32", "float64":
		return "0.0"
	case "complex64", "complex128":
		return "0"
	case "error":
		return "nil"
	}

	// 判定不可
	return fmt.Sprintf("%s{}.%s", structTypeName, rcv.name)
}

func (rcv fieldNameAndType) getGetterMethodName() string {
	return "Get" + strcase.ToUpperCamel(rcv.name);
}


var (
	// constructorTmpl はコンストラクタのテンプレート
	constructorTmpl = template.Must(template.New("constructor").Parse(`
// New{{ .StructName }} is constructor for {{ .StructName }}.
func New{{ .StructName }}({{ .Argments }}) *{{ .StructName }} {
				return &{{ .StructName }}{ {{ .Fields }} }
			}
		`))

	// getterTmpl は値のGetterメソッドのテンプレート
	getterTmpl = template.Must(template.New("getter").Parse(`
// {{ .MethodName }} returns {{ .FieldName }}.
func (rcv *{{ .StructName }}) {{ .MethodName }}() {{ .FieldType }} {
				if rcv == nil {
					// return zero value
					return {{ .ZeroValue }}
				}
				return rcv.{{ .FieldName }}
			}
		`))
)

// constructorTmplParams はコンストラクタのテンプレートで使用するパラメーター
type constructorTmplParams struct {
	StructName string
	Argments string
	Fields  string
}

// getterTmplParam はGetterメソッドのテンプレートで使用するパラメーター
type getterTmplParam struct {
	StructName string
	MethodName string
	FieldType  string
	ZeroValue  string
	FieldName  string
}
