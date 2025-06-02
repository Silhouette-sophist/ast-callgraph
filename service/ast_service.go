package service

import (
	"ast-callgraph/vs"
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"golang.org/x/mod/modfile"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type AstTransverseParam struct {
	Directory string
	GoModPath *string
}

type AstTransverseInfo struct {
	RootPkg string
	*ModFileInfo
	StructInfoMap map[string][]*vs.StructInfo
}

type ModFileInfo struct {
	RootPkg  string
	ModPath  string
	DepsMods []*DepsMod
}

type DepsMod struct {
	Pkg string
	Tag string
}

// TransverseDirectory 遍历指定目录
func TransverseDirectory(ctx context.Context, param *AstTransverseParam) (*AstTransverseInfo, error) {
	// 1.匹配go.mod文件内容
	modFileInfo, err := ParseModFile(ctx, param)
	if err != nil || modFileInfo == nil {
		return nil, errors.New("invalid go mod path")
	}
	// 2.构造返回值
	astTransverseInfo := &AstTransverseInfo{
		RootPkg:       modFileInfo.RootPkg,
		ModFileInfo:   modFileInfo,
		StructInfoMap: make(map[string][]*vs.StructInfo),
	}
	// 3.遍历文件目录下所有内容
	if err := filepath.Walk(param.Directory, func(path string, info fs.FileInfo, err error) error {
		// 错误是否传播
		if err != nil {
			return err
		}
		// 目录是否遍历
		if info.IsDir() {
			return nil
		}
		// 分析有效文件
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			fileSet := token.NewFileSet()
			fileContent, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			astFile, err := parser.ParseFile(fileSet, path, fileContent, parser.ParseComments)
			if err != nil {
				return err
			}
			// a.基于路径分析包名
			currentPkg := deductPkgFromPath(modFileInfo, path)
			// b.遍历节点
			visitor := &vs.FileStructVisitor{
				RootPkg:    modFileInfo.RootPkg,
				CurrentPkg: currentPkg,
			}
			ast.Walk(visitor, astFile)
			// c.数据采集
			for s, infos := range visitor.StructInfoMap {
				astTransverseInfo.StructInfoMap[s] = append(astTransverseInfo.StructInfoMap[s], infos...)
			}
		}
		return err
	}); err != nil {
		return nil, err
	}
	return astTransverseInfo, nil
}

func deductPkgFromPath(info *ModFileInfo, path string) string {
	return ""
}

func ParseModFile(ctx context.Context, param *AstTransverseParam) (*ModFileInfo, error) {
	var goModPath string
	if param.GoModPath == nil {
		goModPath = "go.mod"
	} else {
		goModPath = *param.GoModPath
	}
	modFileContent, err := os.ReadFile(goModPath)
	if err != nil {
		log.Fatalf("Error reading go.mod: %v", err)
		return nil, err
	}
	// 解析 go.mod 文件
	modFile, err := modfile.Parse("go.mod", modFileContent, nil)
	if err != nil {
		log.Fatalf("Error parsing go.mod: %v", err)
		return nil, err
	}
	if modFile != nil {
		return nil, err
	}
	m := &ModFileInfo{
		RootPkg:  modFile.Module.Mod.Path,
		ModPath:  goModPath,
		DepsMods: make([]*DepsMod, 0),
	}
	for _, require := range modFile.Require {
		m.DepsMods = append(m.DepsMods, &DepsMod{
			Pkg: require.Mod.Path,
			Tag: require.Mod.Version,
		})
	}
	return m, nil
}
