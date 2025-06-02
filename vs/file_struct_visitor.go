package vs

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

const (
	pkgNameFormat = "%s.%s"
)

type FileStructVisitor struct {
	RootPkg        string
	CurrentPkg     string
	FSet           token.FileSet
	File           string
	RFilePath      string
	RawContent     []string
	ImportedPkgMap map[string]string
	StructInfoMap  map[string][]*StructInfo
	VarMap         map[string]*Var
}

type Var struct {
	Type             string
	Name             string
	Hash             string
	NoName           bool
	IsPointer        bool
	ContextFieldName string
	StartPos         int
	EndPos           int
}

type StructInfo struct {
	Repo           string
	Pkg            string
	File           string
	Name           string
	TypeName       string
	StartLine      int
	EndLine        int
	Content        string
	DepsStructInfo map[string]map[string]StructIndex
}

type StructIndex struct {
	Pkg  string
	Name string
}

func (f *FileStructVisitor) Visit(node ast.Node) ast.Visitor {
	switch n := node.(type) {
	case *ast.GenDecl:
		for _, spec := range n.Specs {
			if importSpec, ok := spec.(*ast.ImportSpec); ok {
				f.CollectFileImportPkg(importSpec)
			} else if valueSpec, ok := spec.(*ast.ValueSpec); ok {
				f.CollectFileGlobalPkgVars(valueSpec)
			}
		}
	case *ast.TypeSpec:
		f.collectStructAndDeps(n)
	}
	return f
}

// CollectFileImportPkg 收集文件导入包信息
func (f *FileStructVisitor) CollectFileImportPkg(spec *ast.ImportSpec) {
	pkgName := ""
	pkgPath := strings.Trim(spec.Path.Value, "\"")
	// 导入别名
	if spec.Name != nil {
		pkgName = spec.Name.Name
	} else {
		// 系统包还是外部包
		if strings.Contains(pkgPath, "/") {
			pkgName = pkgPath[strings.LastIndex(pkgPath, "/")+1:]
		} else {
			pkgName = pkgPath
		}
	}
	f.ImportedPkgMap[pkgName] = pkgPath
}

// CollectFileGlobalPkgVars 采集文件包名结构体数据
func (f *FileStructVisitor) CollectFileGlobalPkgVars(spec *ast.ValueSpec) {
	for _, name := range spec.Names {
		if name.Obj != nil && name.Obj.Decl != nil {
			if valueSpec, ok := name.Obj.Decl.(*ast.ValueSpec); ok && valueSpec.Values != nil {
				if lit, ok := valueSpec.Values[0].(*ast.CompositeLit); ok {
					shortPkg := ""
					typeName := ""
					if expr, ok := lit.Type.(*ast.SelectorExpr); ok {
						shortPkg = expr.X.(*ast.Ident).Name
						typeName = expr.Sel.Name
					} else if expr, ok := lit.Type.(*ast.Ident); ok {
						typeName = expr.Name
					}
					f.VarMap[name.Name] = &Var{
						Type:     f.getFullTypeName(shortPkg, typeName, false),
						Name:     name.Name,
						NoName:   false,
						StartPos: f.FSet.Position(name.Pos()).Offset,
						EndPos:   f.FSet.Position(name.End()).Offset,
					}
				}
			}
		}
	}
}

func (f *FileStructVisitor) collectStructAndDeps(n *ast.TypeSpec) {
	if structType, ok := n.Type.(*ast.StructType); ok {
		typeName := f.getFullTypeName(n.Name.Name, n.Name.Name, false)
		startLine := f.FSet.Position(n.Pos()).Line
		endLine := f.FSet.Position(n.End()).Line
		currentStructInfo := &StructInfo{
			Repo:      f.RootPkg,
			Pkg:       f.CurrentPkg,
			File:      f.RFilePath,
			Name:      n.Name.Name,
			TypeName:  typeName,
			StartLine: startLine,
			EndLine:   endLine,
			Content:   strings.Join(f.RawContent[startLine-1:endLine], "\n"),
		}
		f.StructInfoMap[currentStructInfo.Pkg] = append(f.StructInfoMap[currentStructInfo.Pkg], currentStructInfo)
		if structType.Fields != nil {
			for _, field := range structType.Fields.List {
				var shortPkg, shortName string
				expr := field.Type
				if arrayType, ok := expr.(*ast.ArrayType); ok {
					shortPkg, shortName = parseStarExprIfNeed(arrayType.Elt, false)
				} else if mapType, ok := expr.(*ast.MapType); ok {
					shortPkg, shortName = parseStarExprIfNeed(mapType.Value, false)
				} else {
					shortPkg, shortName = parseStarExprIfNeed(expr, false)
				}
				if shortName == "" {
					continue
				}
				completePkg := f.CurrentPkg
				if shortPkg != "" {
					if pkg, ok := f.ImportedPkgMap[shortPkg]; ok {
						completePkg = pkg
					}
				}
				if _, ok := currentStructInfo.DepsStructInfo[completePkg]; !ok {
					currentStructInfo.DepsStructInfo[completePkg] = make(map[string]StructIndex)
				}
				currentStructInfo.DepsStructInfo[completePkg][shortName] = StructIndex{
					Pkg:  completePkg,
					Name: shortName,
				}
			}
		}
	}
}

func parseStarExprIfNeed(expr ast.Expr, includeBase bool) (shortPkg string, shortName string) {
	if starExpr, ok := expr.(*ast.StarExpr); ok {
		shortPkg, shortName = parseSimpleExpr(starExpr.X, includeBase)
	} else {
		shortPkg, shortName = parseSimpleExpr(expr, includeBase)
	}
	return
}

func parseSimpleExpr(expr ast.Expr, includeBase bool) (shortPkg string, shortName string) {
	if selectorExpr, ok := expr.(*ast.SelectorExpr); ok {
		shortPkg = selectorExpr.X.(*ast.Ident).Name
		shortName = selectorExpr.Sel.Name
	} else if ident, ok := expr.(*ast.Ident); ok {
		if includeBase {
			shortName = ident.Name
		} else {
			if ident.Obj == nil || ident.Obj.Decl == nil {
				return
			}
			if _, ok := ident.Obj.Decl.(*ast.TypeSpec); ok {
				shortPkg = ident.Name
			}
		}
	}
	return
}

func (f *FileStructVisitor) getFullTypeName(shortPkg string, typeName string, receiver bool) string {
	if receiver {
		return fmt.Sprintf(pkgNameFormat, f.CurrentPkg, typeName)
	} else if pkgPath, ok := f.ImportedPkgMap[shortPkg]; ok {
		return fmt.Sprintf(pkgNameFormat, pkgPath, typeName)
	} else if isBasicType(typeName) {
		return typeName
	} else {
		return fmt.Sprintf(pkgNameFormat, f.CurrentPkg, typeName)
	}
}

func isBasicType(name string) bool {
	switch name {
	case "bool":
		return true
	case "string":
		return true
	case "int", "int8", "int16", "int32", "int64":
		return true
	case "unit", "unit8", "unit16", "unit32", "unit64":
		return true
	case "float32", "float64":
		return true
	case "complex64", "complex128":
		return true
	case "error":
		return true
	default:
		return false
	}
}
