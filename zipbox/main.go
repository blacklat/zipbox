package main

import (
	"fmt"
	"go/build"
	"go/ast"
	"go/parser"
	"go/token"
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"path/filepath"
	"os"
	"flag"
)

type ZIP_BOX struct {
	quiet			bool
	binaryFile		string
	packages		[]*build.Package
	zipboxImport	struct{
		name		string
		path		string
	}
}

type flagStrSlc		[]string

func (f *flagStrSlc) Set(value string) error {
    value = strings.TrimSpace(value)
    if value == "" {
        return fmt.Errorf("invalid value")
    }
    *f = append(*f, value)
    return nil
}

func (f *flagStrSlc) String() string {
    return fmt.Sprintf("%s", *f)
}

func main() {
	var (
		opt_search_path	flagStrSlc
		opt_build_tags	flagStrSlc
		opt_quiet       = flag.Bool("quiet", false, "quiet mode")
		opt_bin_file    = flag.String("bin-file", "", "binary file path")

		opt_imp_name	= flag.String("import-name", "zipbox", "zipbox package name")
		opt_imp_path	= flag.String("import-path", "github.com/blacklat/zipbox", "zipbox package path")
	)

	flag.Var(&opt_search_path, "search-path", "search path")
    flag.Var(&opt_search_path, "sp", "search path")

    flag.Var(&opt_build_tags, "build-tags", "build tags")
    flag.Var(&opt_build_tags, "bt", "build tags")

    flag.BoolVar(opt_quiet, "q", false, "quiet mode")

	flag.StringVar(opt_imp_name, "in", "zipbox", "zipbox package name")
	flag.StringVar(opt_imp_path, "ip", "github.com/blacklat/zipbox", "zipbox package path")

    flag.StringVar(opt_bin_file, "bf", "", "binary file path")

	var usage = func() {
        fmt.Fprintf(os.Stderr, `usage: %s [options]

options:
 -q,  --quiet       > suppresses all output content except for errors
 -sp, --search-path > directory path of your go files that call ZipBox
 -bt, --build-tags
 -in, --import-name > the name of your imported zipbox package
 -ip, --import-path > the path of your imported zipbox package
 -bf, --bin-file    > path of your go binary that will be appended with ZipBox

`, os.Args[0])
		os.Exit(1)
    }
	flag.Usage = usage
    flag.Parse()

	*opt_imp_name = strings.TrimSpace(*opt_imp_name)
	*opt_imp_path = strings.TrimSpace(*opt_imp_path)
	*opt_imp_path = strings.TrimLeft(*opt_imp_path, `"`)
	*opt_imp_path = strings.TrimRight(*opt_imp_path, `"`)

	if *opt_bin_file == "" || *opt_imp_path == "" || *opt_imp_name == "" {
        usage()
    }

	*opt_imp_path = fmt.Sprint(`"`, *opt_imp_path, `"`)

	if len(opt_search_path) == 0 {
		opt_search_path = append(opt_search_path, ".")
	}

	zb := &ZIP_BOX{quiet: *opt_quiet}
	zb.zipboxImport.name = *opt_imp_name
	zb.zipboxImport.path = *opt_imp_path

	if file, err := filepath.Abs(*opt_bin_file); err != nil {
		zb.Err("unable to find absolute path: %v\n", err)
	} else {
		zb.binaryFile = file
	}

	for i := range opt_search_path {
		if err := zb.GetPackage(opt_search_path[i], opt_build_tags); err != nil {
			zb.Err("%v\n", err)
		}
	}

	if err := zb.Append(); err != nil {
		zb.Err("%v\n", err)
	}
}

func (z *ZIP_BOX) GetPackage(path string, tags []string) error {
	pwd, err := os.Getwd()
	if err != nil {
		return err
	}
	pkg, err := build.Import(path, pwd, 0)
	if err != nil {
        return err
    }
	if len(tags) > 0 {
		pkg.AllTags = tags
	}
	z.packages = append(z.packages, pkg)
	return nil
}

func (z *ZIP_BOX) Append() error {
	buffer_zip := new(bytes.Buffer)
	defer buffer_zip.Reset()
	writer_zip := zip.NewWriter(buffer_zip)
	if info, err := os.Stat(z.binaryFile); err != nil {
		return err
	} else {
		writer_zip.SetOffset(info.Size())
	}
	for _, pkg := range z.packages {
		zipBoxes := z.findZipBoxes(pkg)
		if len(zipBoxes) == 0 {
			z.Out("no calls to %s.GetBox() found in search-path -> %v", z.zipboxImport.name, pkg.ImportPath)
			continue
		}
		for zbox := range zipBoxes {
			zboxPath := filepath.Clean(filepath.Join(pkg.Dir, zbox))
			filepath.Walk(zboxPath, func(path string, info os.FileInfo, err error) error {
				if info == nil {
					return fmt.Errorf("file or directory not found: %v", path)
				}
				zipFileName := filepath.Join(strings.Replace(zbox, `/`, `-`, -1), strings.TrimPrefix(path, zboxPath))
				if info.IsDir() {
					header := &zip.FileHeader{
						Name:     zipFileName,
						Comment:  "dir",
					}
					header.SetModTime(info.ModTime())
					if _, err := writer_zip.CreateHeader(header); err != nil {
						return fmt.Errorf("unable to create zip header: %v", err)
					}
					return nil
				}
				zipFileHeader, err := zip.FileInfoHeader(info)
				if err != nil {
					return fmt.Errorf("unable to create zip file header: %v", err)
				}
				zipFileHeader.Name = zipFileName
				writer_file, err := writer_zip.CreateHeader(zipFileHeader)
				if err != nil {
					return fmt.Errorf("unable to create file in zip buffer: %v", err)
				}
				if data, err := os.Open(path); err != nil {
					return fmt.Errorf("unable to open file for read: %v", err)
				} else {
					if _, err := io.Copy(writer_file, data); err != nil {
						return fmt.Errorf("unable to copy file contents to zip buffer: %v", err)
					}
				}
				return nil
			})
		}
	}
	if err := writer_zip.Close(); err != nil {
		return fmt.Errorf("unable to close zip buffer: %v", err)
	}
	if file, err := os.OpenFile(z.binaryFile, os.O_WRONLY, os.ModeAppend); err != nil {
		return err
	} else {
		defer file.Close()
		if _, err := file.Seek(0, 2); err != nil {
			return err
		}
		if _, err := buffer_zip.WriteTo(file); err != nil {
			return fmt.Errorf("unable to write to zip buffer: %v", err)
		}
	}
	return nil
}

func (z *ZIP_BOX) findZipBoxes(pkg *build.Package) map[string]bool {
	var zipBoxes = make(map[string]bool)
	filenames := make([]string, 0, len(pkg.GoFiles)+len(pkg.CgoFiles))
	filenames = append(filenames, pkg.GoFiles...)
	filenames = append(filenames, pkg.CgoFiles...)
	for _, filename := range filenames {
		fullpath := filepath.Join(pkg.Dir, filename)
		z.Out("scanning file: %q\n", fullpath)
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, fullpath, nil, 0)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		var myPkgName string
		for _, imp := range f.Imports {
			if imp.Path.Value == z.zipboxImport.path {
				if imp.Name != nil {
					myPkgName = imp.Name.Name
				} else {
					myPkgName = z.zipboxImport.name
				}
				break
			}
		}
		if myPkgName == "" || myPkgName == "_" {
			continue
		}
		var (
			isValidIdent bool
			isValidBasicLit bool
			tokenPos token.Pos
			isValidVar = make(map[string]bool)
		)
		ast.Inspect(f, func(node ast.Node) bool {
			if node == nil {
				return false
			}
			switch x := node.(type) {
			case *ast.AssignStmt:
				var assign = node.(*ast.AssignStmt)
				name, found := assign.Lhs[0].(*ast.Ident)
				if found {
					composite, first := assign.Rhs[0].(*ast.CompositeLit)
					if first {
						pkgSelector, second := composite.Type.(*ast.SelectorExpr)
						if second {
							packageName, third := pkgSelector.X.(*ast.Ident)
							if third && packageName.Name == myPkgName {
								isValidVar[name.Name] = true
								z.Out("zipbox variable found: %q\n", name.Name)
							}
						}
					}
				}
			case *ast.Ident:
				if isValidIdent || myPkgName == "." {
					isValidIdent = false
					if x.Name == "Get" {
						isValidBasicLit = true
						tokenPos = x.Pos()
					}
				} else {
					if x.Name == myPkgName || isValidVar[x.Name] {
						isValidIdent = true
					}
				}
			case *ast.BasicLit:
				if isValidBasicLit {
					if x.Kind == token.STRING {
						isValidBasicLit = false
						name := x.Value[1 : len(x.Value)-1]
						zipBoxes[name] = true
						z.Out("zipbox found: %q\n", name)
					} else {
						z.Err("%s:%d: argument must be a literal string\n",
							fset.Position(tokenPos).Filename, fset.Position(tokenPos).Line,
						)
						return false
					}
				}
			default:
				if isValidIdent {
					isValidIdent = false
				}
				if isValidBasicLit {
					z.Err("%s:%d: argument must be a literal string\n",
						fset.Position(tokenPos).Filename, fset.Position(tokenPos).Line,
					)
					return false
				}
			}
			return true
		})
	}
	return zipBoxes
}

func (z *ZIP_BOX) Err(f string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, "[ERROR] " + f, v...)
	os.Exit(1)
}

func (z *ZIP_BOX) Out(f string, v ...interface{}) {
	if z.quiet {
		return
	}
	fmt.Fprintf(os.Stdout, "[INFO] " + f, v...)
}
