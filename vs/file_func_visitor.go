package vs

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

var keywords = map[string]struct{}{
	"break": {}, "case": {}, "chan": {}, "const": {},
	"continue": {}, "default": {}, "defer": {}, "delete": {},
	"else": {}, "fallthrough": {}, "for": {}, "func": {},
	"go": {}, "goto": {}, "if": {}, "import": {},
	"interface": {}, "map": {}, "package": {}, "range": {},
	"return": {}, "select": {}, "struct": {}, "switch": {},
	"type": {}, "var": {},
}

var builtInFunctions = map[string]struct{}{
	"append": {}, "cap": {}, "close": {}, "copy": {},
	"delete": {}, "len": {}, "make": {}, "new": {},
	"panic": {}, "print": {}, "println": {}, "real": {},
	"recover": {}, "complex": {}, "imag": {},
}

var typeConversions = map[string]struct{}{
	"bool": {}, "byte": {}, "complex64": {}, "complex128": {},
	"float32": {}, "float64": {}, "int": {}, "int8": {},
	"int16": {}, "int32": {}, "int64": {}, "rune": {},
	"string": {}, "uint": {}, "uint8": {}, "uint16": {},
	"uint32": {}, "uint64": {}, "uintptr": {},
}

type FileFuncVisitor struct {
	FileStructVisitor
	FuncMap map[string]*GoFunc
}

type GoFunc struct {
	Repo        string
	Pkg         string
	File        string
	RFile       string
	Name        string
	RecvType    *Var
	Params      []*Var
	Results     []*Var
	Begin       token.Position
	End         token.Position
	Content     string
	CalleeInfos []*CalleeInfo
	TmpVars     map[string]*Var
}

type CalleeInfo struct {
	Pkg      string
	File     string
	Name     string
	Begin    token.Position
	End      token.Position
	Receiver *string
}

func (f *FileFuncVisitor) Visit(node ast.Node) ast.Visitor {
	var recvField *ast.FieldList
	var funcType *ast.FuncType
	goFunc := &GoFunc{
		Repo:    f.RootPkg,
		Pkg:     f.CurrentPkg,
		File:    f.File,
		TmpVars: make(map[string]*Var),
	}
	switch n := node.(type) {
	case *ast.GenDecl, *ast.TypeSpec:
		return f.FileStructVisitor.Visit(n)
	case *ast.FuncDecl:
		goFunc.Name = n.Name.Name
		if goFunc.Name == "init" {
			goFunc.Name = fmt.Sprintf("init#%d", "xxx")
		}
		goFunc.Begin = f.FSet.Position(n.Pos())
		goFunc.End = f.FSet.Position(n.End())
		f.CollectFuncBasicInfo(goFunc, funcType, recvField)
		f.CollectFuncBodyCaller(goFunc, n.Body)
	case *ast.FuncLit:
		funcType = n.Type
		goFunc.Begin = f.FSet.Position(n.Pos())
		goFunc.End = f.FSet.Position(n.End())
		f.CollectFuncBasicInfo(goFunc, funcType, recvField)
		f.CollectFuncBodyCaller(goFunc, n.Body)

	}
	return f
}

func (f *FileFuncVisitor) CollectFuncBasicInfo(goFunc *GoFunc, funcType *ast.FuncType, recvField *ast.FieldList) {
	if funcType == nil {
		return
	}
	if recvField == nil {
		f.handleFieldList(recvField.List, func(v *Var) {
			goFunc.RecvType = v
		}, true)
	}
	if funcType.Params != nil {
		f.handleFieldList(funcType.Params.List, func(v *Var) {
			goFunc.Params = append(goFunc.Params, v)
		}, false)
	}
	if funcType.Results != nil {
		retCnt := 0
		f.handleFieldList(funcType.Results.List, func(v *Var) {
			if v.Name == "" {
				v.Name = fmt.Sprintf("rt%d", retCnt)
				retCnt++
			}
			goFunc.Results = append(goFunc.Results, v)
		}, false)
	}
	f.FuncMap[goFunc.Name] = goFunc
}

func (f *FileFuncVisitor) handleFieldList(list []*ast.Field, handle func(v *Var), isRecv bool) {
	for _, field := range list {
		var typeStr string
		switch t := field.Type.(type) {
		case *ast.Ident, *ast.SelectorExpr:
			shortPkg, name := parseSimpleExpr(t, true)
			typeName := f.getFullTypeName(shortPkg, name, isRecv)
			typeStr = typeName
		case *ast.StarExpr:
			shortPkg, name := parseStarExprIfNeed(t.X, true)
			typeName := f.getFullTypeName(shortPkg, name, isRecv)
			typeStr = "*" + typeName
		case *ast.ArrayType:
			shortPkg, name := parseStarExprIfNeed(t.Elt, true)
			typeName := f.getFullTypeName(shortPkg, name, isRecv)
			typeStr = "[]" + typeName
		case *ast.MapType:
			shortPkg, name := parseStarExprIfNeed(t.Value, true)
			typeName := f.getFullTypeName(shortPkg, name, isRecv)
			if ident, ok := t.Key.(*ast.Ident); ok {
				typeStr = fmt.Sprintf("map[%s]%s", ident.Name, typeName)
			} else {
				typeStr = fmt.Sprintf("map[]%s", typeName)
			}
		default:
			typeStr = "unknown"
		}
		isPointer := strings.HasPrefix(typeStr, "*")
		startPos := f.FSet.Position(field.Pos()).Offset
		endPos := f.FSet.Position(field.End()).Offset
		if len(field.Names) > 0 {
			for _, name := range field.Names {
				v := &Var{
					Type:      typeStr,
					Name:      name.Name,
					NoName:    name.Name == "",
					IsPointer: isPointer,
					StartPos:  startPos,
					EndPos:    endPos,
				}
				handle(v)
			}
		} else {
			handle(&Var{
				Type:      typeStr,
				NoName:    true,
				IsPointer: isPointer,
				StartPos:  startPos,
				EndPos:    endPos,
			})
		}
	}
}

func (f *FileFuncVisitor) CollectFuncBodyCaller(goFunc *GoFunc, body *ast.BlockStmt) {
	if goFunc == nil || body == nil {
		return
	}
	ast.Inspect(body, func(nx ast.Node) bool {
		if callExpr, ok := nx.(*ast.CallExpr); ok {
			// 1.函数调用
			f.handleCallExpr(callExpr, goFunc)
		} else if assignStmt, ok := nx.(*ast.AssignStmt); ok {
			// 1.函数内局部变量赋值语句
			f.handleFuncVarsAssign(assignStmt, goFunc)
		} else if decl, ok := nx.(*ast.GenDecl); ok && decl.Tok == token.VAR {
			// 1.函数内局部变量声明
			f.handleFuncVarDecl(decl, goFunc)
		}
		return true
	})
}

func (f *FileFuncVisitor) handleCallExpr(expr *ast.CallExpr, goFunc *GoFunc) {
	if selExpr, ok := expr.Fun.(*ast.SelectorExpr); ok {
		// 选择器调用
		f.handleSelectorExprCall(selExpr, goFunc)
	} else if ident, ok := expr.Fun.(*ast.Ident); ok {
		// 函数名调用
		f.handleIdentCall(ident, goFunc)
	}
	// 函数参数调用采集
	f.handleFuncArgsCall(expr, goFunc)
}

func (f *FileFuncVisitor) handleFuncArgsCall(callExpr *ast.CallExpr, goFunc *GoFunc) {
	for _, arg := range callExpr.Args {
		if ident, ok := arg.(*ast.Ident); ok {
			if ident.Obj != nil && ident.Obj.Decl != nil {
				if _, ok := ident.Obj.Decl.(*ast.FuncDecl); ok {
					f.handleIdentCall(ident, goFunc)
				}
			}
		} else if selectorExpr, ok := arg.(*ast.SelectorExpr); ok {
			if selectorExpr.Sel.Obj != nil && selectorExpr.Sel.Obj.Decl != nil {
				if _, ok := selectorExpr.Sel.Obj.Decl.(*ast.FuncDecl); ok {
					f.handleSelectorExprCall(selectorExpr, goFunc)
				}
			}
		}
	}
}

func (f *FileFuncVisitor) handleIdentCall(ident *ast.Ident, goFunc *GoFunc) {
	identName := ident.Name
	if _, ok := keywords[identName]; !ok {
		if _, ok := builtInFunctions[identName]; !ok {
			if _, ok := typeConversions[identName]; !ok {
				goFunc.CalleeInfos = append(goFunc.CalleeInfos, &CalleeInfo{
					Pkg:   goFunc.Pkg,
					File:  goFunc.RFile,
					Begin: f.FSet.Position(ident.Pos()),
					End:   f.FSet.Position(ident.End()),
				})
			}
		}
	}
}

func (f *FileFuncVisitor) handleSelectorExprCall(selExpr *ast.SelectorExpr, goFunc *GoFunc) {
	if ident, ok := selExpr.X.(*ast.Ident); ok {
		shortPkgName := ident.Name
		if goFunc.RecvType != nil && goFunc.RecvType.Name == shortPkgName {
			info := &CalleeInfo{
				Pkg:   goFunc.Pkg,
				File:  goFunc.RFile,
				Name:  selExpr.Sel.Name,
				Begin: f.FSet.Position(ident.Pos()),
				End:   f.FSet.Position(ident.End()),
			}
			if goFunc.RecvType.Type != "" {
				info.Receiver = &goFunc.RecvType.Type
			}
			goFunc.CalleeInfos = append(goFunc.CalleeInfos, info)
		} else if pkgInfo, ok := f.ImportedPkgMap[shortPkgName]; ok {
			if strings.HasPrefix(pkgInfo, f.RootPkg) {
				goFunc.CalleeInfos = append(goFunc.CalleeInfos, &CalleeInfo{
					Pkg:   pkgInfo,
					File:  goFunc.RFile,
					Name:  selExpr.Sel.Name,
					Begin: f.FSet.Position(ident.Pos()),
					End:   f.FSet.Position(ident.End()),
				})
			}
		} else if pkgVar, ok := f.VarMap[shortPkgName]; ok {
			goFunc.CalleeInfos = append(goFunc.CalleeInfos, &CalleeInfo{
				Pkg:   pkgVar.Type,
				File:  goFunc.RFile,
				Name:  selExpr.Sel.Name,
				Begin: f.FSet.Position(ident.Pos()),
				End:   f.FSet.Position(ident.End()),
			})
		} else {
			for _, param := range goFunc.Params {
				if param.Name == shortPkgName {
					goFunc.CalleeInfos = append(goFunc.CalleeInfos, &CalleeInfo{
						Pkg:   goFunc.Pkg,
						File:  goFunc.RFile,
						Name:  selExpr.Sel.Name,
						Begin: f.FSet.Position(ident.Pos()),
						End:   f.FSet.Position(ident.End()),
					})
				}
			}
		}
	}
}

func (f *FileFuncVisitor) handleFuncVarDecl(decl *ast.GenDecl, goFunc *GoFunc) {
	for _, spec := range decl.Specs {
		if valueSpec, ok := spec.(*ast.ValueSpec); ok {
			shortPkgName := ""
			typeName := ""
			if ident, ok := valueSpec.Type.(*ast.Ident); ok {
				typeName = ident.Name
			} else if selectorExpr, ok := valueSpec.Type.(*ast.SelectorExpr); ok {
				shortPkgName = selectorExpr.X.(*ast.Ident).Name
				typeName = selectorExpr.Sel.Name
			}
			for _, name := range valueSpec.Names {
				goFunc.TmpVars[name.Name] = &Var{
					Name:   name.Name,
					Type:   f.getFullTypeName(shortPkgName, typeName, false),
					NoName: false,
				}
			}
		}
	}
}

func (f *FileFuncVisitor) handleFuncVarsAssign(stmt *ast.AssignStmt, goFunc *GoFunc) {
	var shortPkgName, typeName string
	if ident, ok := stmt.Rhs[0].(*ast.CompositeLit); ok {
		if a, ok := ident.Type.(*ast.Ident); ok {
			typeName = a.Name
		} else if a, ok := ident.Type.(*ast.SelectorExpr); ok {
			shortPkgName = a.X.(*ast.Ident).Name
			typeName = a.Sel.Name
		}
	} else if selectorExpr, ok := stmt.Rhs[0].(*ast.SelectorExpr); ok {
		if a, ok := selectorExpr.X.(*ast.Ident); ok {
			shortPkgName = a.Name
			typeName = selectorExpr.Sel.Name
		}
	}
	for _, lh := range stmt.Lhs {
		if ident, ok := lh.(*ast.Ident); ok {
			goFunc.TmpVars[ident.Name] = &Var{
				Name:   ident.Name,
				Type:   f.getFullTypeName(shortPkgName, typeName, false),
				NoName: false,
			}
		}
	}
}
