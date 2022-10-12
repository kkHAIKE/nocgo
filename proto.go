package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
)

type Parameter struct {
	Name    string
	Size    int
	IsFloat bool
}

type Function struct {
	Name string
	Args []*Parameter
	Ret  *Parameter
}

func (f *Function) ArgsSize() (ret int) {
	for _, v := range f.Args {
		ret += v.Size
	}
	if f.Ret != nil {
		ret += f.Ret.Size
	}
	return
}

type Functions []*Function

func paramParse(args *ast.FieldList) (ret []*Parameter, err error) {
	// void result only
	if args == nil {
		return []*Parameter{nil}, nil
	}
	for _, v := range args.List {
		if len(v.Names) == 0 {
			err = errors.New("need parameter name")
			return
		}

		var sz int
		var fp bool
		switch t := v.Type.(type) {
		case *ast.StarExpr:
			// pointer
			sz = 8
		case *ast.SelectorExpr:
			// unsafe.Pointer
			if n, ok := t.X.(*ast.Ident); ok && n.Name == "unsafe" && t.Sel.Name == "Pointer" {
				sz = 8
			}
		case *ast.Ident:
			switch t.Name {
			case "int8", "uint8", "byte", "bool":
				sz = 1
			case "int16", "uint16":
				sz = 2
			case "float32":
				sz, fp = 4, true
			case "int32", "uint32", "rune":
				sz = 4
			case "float64":
				sz, fp = 8, true
			case "int64", "uint64", "uintptr", "int", "Pointer":
				sz = 8
			}
		}
		if sz == 0 {
			err = fmt.Errorf("unsupport parameter: %v", v.Type)
			return
		}
		for _, name := range v.Names {
			ret = append(ret, &Parameter{
				Name:    name.Name,
				Size:    sz,
				IsFloat: fp,
			})
		}
	}
	return
}

func protoParse(fpath string) (ret Functions, pkg string, err error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, fpath, nil, 0)
	if err != nil {
		return
	}

	pkg = f.Name.Name

	for _, v := range f.Decls {
		fd, ok := v.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fd.Recv != nil {
			err = fmt.Errorf("not support method: %s", fd.Name.Name)
			return
		}
		if fd.Type.Results.NumFields() > 1 {
			err = fmt.Errorf("not support multi results: %s", fd.Name.Name)
			return
		}
		args, err1 := paramParse(fd.Type.Params)
		if err1 != nil {
			err = err1
			return
		}
		res, err1 := paramParse(fd.Type.Results)
		if err1 != nil {
			err = err1
			return
		}
		ret = append(ret, &Function{
			Name: fd.Name.Name,
			Args: args,
			Ret:  res[0],
		})
	}

	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Name < ret[j].Name
	})
	return
}
