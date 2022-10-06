package expander

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"strings"

	"github.com/fatih/structtag"
)

type Field struct {
	Name    string
	Type    string
	TagName string
}

func parseField(t ast.Expr) (string, error) {
	switch t := t.(type) {
	case *ast.StarExpr:
		return "", fmt.Errorf("unexpected star expr in switch")
	case *ast.ArrayType:
		switch t2 := t.Elt.(type) {
		case *ast.Ident:
			return "[]" + t2.String(), nil
		default:
			return "", fmt.Errorf("unexpected slice of unknown in switch")
		}
	case *ast.StructType:
		return "", fmt.Errorf("unexpected struct type in switch")
	case *ast.Ident:
		return t.String(), nil
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", t.X, t.Sel), nil
	case *ast.MapType:
		mapValue, err := parseField(t.Value)
		if err != nil {
			return "", fmt.Errorf("parsing map value: %v", err)
		}
		return fmt.Sprintf("map[%s]%s", t.Key, mapValue), nil
	case *ast.InterfaceType:
		return "", fmt.Errorf("unexpected interface type in switch")
	default:
		return "", fmt.Errorf("unexpected no type found in switch")
	}
}

func Expand(fn string) error {
	var f ast.Node
	f, err := parser.ParseFile(token.NewFileSet(), fn, nil, parser.SpuriousErrors)
	if err != nil {
		return fmt.Errorf("ParseFile error: %v", err)
	}

	update := new(strings.Builder)
	// filter := new(strings.Builder)
	name := "Struct"

	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			name = x.Name
		case *ast.StructType:
			updateFields := []Field{}
			for _, astField := range x.Fields.List {
				field := Field{}
				addField := false

				tags, err := structtag.Parse(strings.Replace(astField.Tag.Value, "`", "", 2))
				if err != nil {
					panic(fmt.Errorf("parsing tags: %v", err))
				}
				for _, tag := range tags.Tags() {
					if tag.Key == "update" {
						addField = true
					}
					if tag.Key == "json" {
						field.TagName = tag.Name
					}
				}

				if addField {
					field.Name = astField.Names[0].Name
					if field.TagName == "" {
						field.TagName = strings.ToLower(field.Name)
					}

					t, err := parseField(astField.Type)
					if err != nil {
						panic(fmt.Errorf("parseField: %v", err))
					}
					field.Type = t

					updateFields = append(updateFields, field)
				}
			}

			if len(updateFields) > 0 {
				update.WriteString("\ntype " + name + "Update struct {\n")
				for _, field := range updateFields {
					update.WriteString(fmt.Sprintf("\t%s\t\t*%s\t\t`json:\"%s,omitempty\" bson:\"%s,omitempty\"`\n", field.Name, field.Type, field.TagName, field.TagName))
				}
				update.WriteString("}")
			}

			return false
		}
		return true
	})

	file, err := os.OpenFile(fn, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(fmt.Errorf("opening file: %v", err))
	}
	w := bufio.NewWriter(file)
	updateText := update.String()
	if len(updateText) > 0 {
		w.WriteString(updateText + "\n")
	}
	w.Flush()

	return nil
}
