package converter

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"go/ast"
	"go/parser"
	"go/token"

	"github.com/fatih/structtag"
)

type Import struct {
	Package string
	Struct  string
}

var Indent = "    "

func getIdent(s string) string {
	switch s {
	case "bool":
		return "boolean"
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64",
		"complex64", "complex128":
		return "number"
	}

	return s
}

func startsWithCapital(s string) bool {
	for _, r := range s {
		return unicode.IsUpper(r)
	}
	return false // would only happen on empty string
}

func writeType(s *strings.Builder, t ast.Expr, depth int, optionalParens bool) (externalImports []Import, internalImports []string, e error) {
	switch t := t.(type) {
	case *ast.StarExpr:
		if optionalParens {
			s.WriteByte('(')
		}
		ei, ii, err := writeType(s, t.X, depth, false)
		externalImports = append(externalImports, ei...)
		internalImports = append(internalImports, ii...)
		if err != nil {
			e = fmt.Errorf("writeType error: %v", err)
			return
		}
		s.WriteString(" | undefined")
		if optionalParens {
			s.WriteByte(')')
		}
	case *ast.ArrayType:
		if v, ok := t.Elt.(*ast.Ident); ok && v.String() == "byte" {
			s.WriteString("string")
			break
		}
		ei, ii, err := writeType(s, t.Elt, depth, true)
		externalImports = append(externalImports, ei...)
		internalImports = append(internalImports, ii...)
		if err != nil {
			e = fmt.Errorf("writeType error: %v", err)
			return
		}
		s.WriteString("[]")
	case *ast.StructType:
		s.WriteString("{\n")
		ei, ii, err := writeFields(s, t.Fields.List, depth+1)
		externalImports = append(externalImports, ei...)
		internalImports = append(internalImports, ii...)
		if err != nil {
			e = fmt.Errorf("writeFields error: %v", err)
			return
		}

		for i := 0; i < depth+1; i++ {
			s.WriteString(Indent)
		}
		s.WriteByte('}')
	case *ast.Ident:
		s.WriteString(getIdent(t.String()))
		if t.Obj == nil && startsWithCapital(t.Name) {

			duplicate := false
			for _, existing := range internalImports {
				if t.Name == existing {
					duplicate = true
				}
			}
			if !duplicate {
				internalImports = append(internalImports, t.Name)
			}

			internalImports = append(internalImports, t.Name)
		}
	case *ast.SelectorExpr:
		longType := fmt.Sprintf("%s.%s", t.X, t.Sel)
		switch longType {
		case "time.Time":
			s.WriteString("string")
		case "decimal.Decimal":
			s.WriteString("number")
		default:
			s.WriteString(t.Sel.String())
			externalImports = append(externalImports, Import{Package: fmt.Sprintf("%s", t.X), Struct: t.Sel.String()})
		}
	case *ast.MapType:
		s.WriteString("{ [key: ")
		ei, ii, err := writeType(s, t.Key, depth, false)
		externalImports = append(externalImports, ei...)
		internalImports = append(internalImports, ii...)
		if err != nil {
			e = fmt.Errorf("writeType error: %v", err)
			return
		}
		s.WriteString("]: ")
		ei, ii, err = writeType(s, t.Value, depth, false)
		externalImports = append(externalImports, ei...)
		internalImports = append(internalImports, ii...)
		if err != nil {
			e = fmt.Errorf("writeType error: %v", err)
			return
		}
		s.WriteByte('}')
	case *ast.InterfaceType:
		s.WriteString("any")
	default:
		err := fmt.Errorf("unhandled: %s, %T", t, t)
		fmt.Println(err)
		e = fmt.Errorf("switch t := t.(type): %v", err)
		return
	}

	return
}

var validJSNameRegexp = regexp.MustCompile(`(?m)^[\pL_][\pL\pN_]*$`)

func validJSName(n string) bool {
	return validJSNameRegexp.MatchString(n)
}

func writeFields(s *strings.Builder, fields []*ast.Field, depth int) (externalImports []Import, internalImports []string, e error) {
	for _, f := range fields {
		optional := false

		var fieldName string
		if len(f.Names) != 0 && f.Names[0] != nil && len(f.Names[0].Name) != 0 {
			fieldName = f.Names[0].Name
		}
		if len(fieldName) == 0 || 'A' > fieldName[0] || fieldName[0] > 'Z' {
			continue
		}

		var name string
		if f.Tag != nil {
			tags, err := structtag.Parse(f.Tag.Value[1 : len(f.Tag.Value)-1])
			if err != nil {
				e = fmt.Errorf("structtag.Parse error: %v", err)
				return
			}

			jsonTag, err := tags.Get("json")
			if err == nil {
				name = jsonTag.Name
				if name == "-" {
					continue
				}

				optional = jsonTag.HasOption("omitempty")
			}
		}

		if len(name) == 0 {
			name = fieldName
		}

		for i := 0; i < depth+1; i++ {
			s.WriteString(Indent)
		}

		quoted := !validJSName(name)

		if quoted {
			s.WriteByte('\'')
		}
		s.WriteString(name)
		if quoted {
			s.WriteByte('\'')
		}

		switch t := f.Type.(type) {
		case *ast.StarExpr:
			optional = true
			f.Type = t.X
		}

		if optional {
			s.WriteByte('?')
		}

		s.WriteString(": ")

		ei, ii, err := writeType(s, f.Type, depth, false)
		externalImports = append(externalImports, ei...)
		internalImports = append(internalImports, ii...)
		if err != nil {
			e = fmt.Errorf("writeType error: %v", err)
			return
		}

		s.WriteString(";\n")
	}

	return
}

type Response struct {
	Interfaces      []string
	ExternalImports []Import
	InternalImports []string
	FullText        string
}

func Convert(fn string) (*Response, error) {
	var f ast.Node
	f, err := parser.ParseFile(token.NewFileSet(), fn, nil, parser.SpuriousErrors)
	if err != nil {
		return nil, fmt.Errorf("ParseExprFrom: %v", err)
	}

	var interfaces []string
	var externalImports []Import
	var internalImports []string

	w := new(strings.Builder)
	name := "MyInterface"

	first := true

	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			name = x.Name
		case *ast.StructType:
			if !first {
				w.WriteString("\n\n")
			}

			interfaces = append(interfaces, name)
			w.WriteString("export interface ")
			w.WriteString(name)
			w.WriteString(" {\n")

			ei, ii, err := writeFields(w, x.Fields.List, 0)
			externalImports = append(externalImports, ei...)
			internalImports = append(internalImports, ii...)
			if err != nil {
				fmt.Printf("writeFields error: %v\n", err)
			}

			w.WriteByte('}')

			first = false
			return false
		}
		return true
	})

	fullText := w.String()

	res := Response{
		Interfaces:      interfaces,
		ExternalImports: externalImports,
		InternalImports: internalImports,
		FullText:        fullText,
	}

	return &res, nil
}
