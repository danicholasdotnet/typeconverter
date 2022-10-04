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
)

type Field struct {
	Name string
	Type string
	Tags []Tag
}

type Tag struct {
	Name  string
	Value string
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
				tags := strings.Split(strings.Replace(astField.Tag.Value, "`", "", 2), " ")
				for _, tag := range tags {
					if tag != "" {
						split := strings.Split(tag, ":")
						name := split[0]
						if name == "update" {
							addField = true
						}

						value := strings.Replace(split[1], `"`, ``, 2)
						field.Tags = append(field.Tags, Tag{
							Name:  name,
							Value: value,
						})
					}
				}

				if addField {
					field.Name = astField.Names[0].Name

					t := astField.Type
					switch t := t.(type) {
					case *ast.StarExpr:
						panic(fmt.Errorf("unexpected star expr: %v", err))
					case *ast.ArrayType:
						switch t2 := t.Elt.(type) {
						case *ast.Ident:
							field.Type = "[]" + t2.String()
						default:
							panic(fmt.Errorf("enexpected slice of unknown: %v", err))
						}
					case *ast.StructType:
						panic(fmt.Errorf("unexpected struct type: %v", err))
					case *ast.Ident:
						field.Type = t.String()
					case *ast.SelectorExpr:
						field.Type = fmt.Sprintf("%s.%s", t.X, t.Sel)
					case *ast.MapType:
						panic(fmt.Errorf("unexpected map type: %v", err))
					case *ast.InterfaceType:
						panic(fmt.Errorf("unexpected interface type: %v", err))
					default:
						panic(fmt.Errorf("unexpected no type found: %v", err))
					}

					updateFields = append(updateFields, field)
				}
			}

			if len(updateFields) > 0 {
				update.WriteString("\ntype " + name + "Update struct {\n")
				for _, field := range updateFields {
					tagString := new(strings.Builder)
					tagString.WriteByte('`')
					for i, tag := range field.Tags {
						if tag.Name != "validate" && tag.Name != "update" {
							if i > 0 {
								tagString.WriteByte(' ')
							}
							tagValue := tag.Value
							if tag.Name == "json" {
								tagValue += ",omitempty"
							}
							tagString.WriteString(tag.Name + `:"` + tagValue + `"`)
						}
					}
					tagString.WriteByte('`')
					update.WriteString(fmt.Sprintf("\t%s\t\t*%s\t\t%s\n", field.Name, field.Type, tagString.String()))
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
