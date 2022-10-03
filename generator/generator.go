package generator

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gitlab.wilxite.uk/wilxite-modules/interface-generator/converter"
)

type FolderName = string
type InterfaceName = string
type DefaultExportMap = map[FolderName]InterfaceName

type Generator struct {
	structsDir       string // directory where all the go files with the structs are kept
	defaultExportMap DefaultExportMap
}

func New(structsDir string, defaultExportMap DefaultExportMap) (g *Generator) {
	return &Generator{
		structsDir:       structsDir,
		defaultExportMap: defaultExportMap,
	}
}

func (g *Generator) Loop() {
	// create index file
	mainIndexPath := filepath.Join("src", "index.ts")

	if err := os.Mkdir("src", os.ModePerm); err != nil {
		log.Fatal(fmt.Errorf("creating folder: %v", err))
	}

	mainIndexFile, err := os.Create(mainIndexPath)
	if err != nil {
		log.Fatal(fmt.Errorf("creating file: %v", err))
	}
	mainIndexWriter := bufio.NewWriter(mainIndexFile)

	defaultExports := []string{}

	// loop through each folder
	folders, err := os.ReadDir(g.structsDir)
	if err != nil {
		log.Fatal(fmt.Errorf("reading directory: %v", err))
	}
	for _, folder := range folders {
		if !folder.IsDir() {
			continue
		}

		// create index file
		folderIndexPath := filepath.Join("src", folder.Name(), "index.ts")

		if err := os.Mkdir(filepath.Join("src", folder.Name()), os.ModePerm); err != nil {
			log.Fatal(fmt.Errorf("creating folder: %v", err))
		}

		folderIndexFile, err := os.Create(folderIndexPath)
		if err != nil {
			log.Fatal(fmt.Errorf("creating index file: %v", err))
		}
		folderIndexWriter := bufio.NewWriter(folderIndexFile)

		files, err := os.ReadDir(filepath.Join(g.structsDir, folder.Name()))
		if err != nil {
			log.Fatal(fmt.Errorf("reading structs directory: %v", err))
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}

			path := filepath.Join(g.structsDir, folder.Name(), file.Name())

			// convert go struct to ts interface
			res, err := converter.Convert(path)
			if err != nil {
				fmt.Printf("converting structs: %v\n", err)
			}
			interfaces := res.Interfaces
			externalImports := res.ExternalImports
			internalImports := res.InternalImports
			fullText := res.FullText

			// make the import map
			importMap := make(map[string][]string)
			for _, i := range externalImports {
				inMap := false
				for pkg, structs := range importMap {
					if pkg == i.Package {
						inMap = true
						inStructs := false
						for _, str := range structs {
							if str == i.Struct {
								inStructs = true
							}
						}
						if !inStructs {
							importMap[pkg] = append(importMap[pkg], i.Struct)
						}
					}
				}
				if !inMap {
					importMap[i.Package] = []string{i.Struct}
				}
			}

			// create the new ts file
			newFile := strings.Replace(strings.Replace(path, "structs", "src", 1), ".go", ".ts", 1)

			newPath, newFN := filepath.Split(newFile)
			if err := os.MkdirAll(newPath, os.ModePerm); err != nil {
				log.Fatal(fmt.Errorf("creating file path: %v", err))
			}

			f, err := os.Create(newFile)
			if err != nil {
				log.Fatal(fmt.Errorf("creating file: %v", err))
			}

			// write the necessary imports to it
			w := bufio.NewWriter(f)
			for pkg, structs := range importMap {
				w.WriteString("import { ")
				for i, str := range structs {
					if i > 0 {
						w.WriteString(", ")
					}
					w.WriteString(str)
				}
				w.WriteString(" } from \"../" + pkg + "\";\n")
			}
			w.WriteString("\n")

			if len(internalImports) > 0 {
				w.WriteString("import { ")
				for i, str := range internalImports {
					if i > 0 {
						w.WriteString(", ")
					}
					w.WriteString(str)
				}
				w.WriteString(" } from \".\";\n")
				w.WriteString("\n")
			}

			// write the generated interface to it
			for _, s := range strings.Split(fullText, "\\n") {
				w.WriteString(s + "\n")
			}
			w.Flush()

			// write imports to folder index file
			defaultExport := ""
			de, ok := g.defaultExportMap[folder.Name()]
			importName := strings.ReplaceAll(strings.Replace("./"+newFN, ".ts", "", 1), string(filepath.Separator), "/")
			folderIndexWriter.WriteString("export { ")
			for i, interfaceName := range interfaces {
				if i > 0 {
					folderIndexWriter.WriteString(", ")
				}
				if ok && de == interfaceName {
					defaultExport = interfaceName
				}
				folderIndexWriter.WriteString(interfaceName)
			}
			folderIndexWriter.WriteString(" } from \"" + importName + "\";\n")
			folderIndexWriter.Flush()

			if defaultExport != "" {
				// write export to folder index file
				folderIndexWriter.WriteString("\nimport { " + defaultExport + " } from \"" + importName + "\";\n")
				folderIndexWriter.WriteString("export default " + defaultExport + ";\n")
				folderIndexWriter.Flush()

				// write import to main index file
				mainIndexWriter.WriteString("import " + defaultExport + " from \"./" + folder.Name() + "\";\n")
				mainIndexWriter.Flush()
				defaultExports = append(defaultExports, defaultExport)
			}
		}
	}

	// write export to main index file
	mainIndexWriter.WriteString("\nexport { ")
	for i, de := range defaultExports {
		if i > 0 {
			mainIndexWriter.WriteString(", ")
		}
		mainIndexWriter.WriteString(de)
	}
	mainIndexWriter.WriteString(" }\n")
	mainIndexWriter.Flush()
}
