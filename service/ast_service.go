package service

import (
	"ast-callgraph/vs"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"golang.org/x/mod/modfile"
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
		hlog.CtxWarnf(ctx, "TransverseDirectory ParseModFile err")
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
			hlog.CtxWarnf(ctx, "TransverseDirectory Walk %s err %v", path, err)
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
				hlog.CtxWarnf(ctx, "TransverseDirectory ReadFile err %v", err)
				return err
			}
			astFile, err := parser.ParseFile(fileSet, path, fileContent, parser.ParseComments)
			if err != nil {
				hlog.CtxWarnf(ctx, "TransverseDirectory ParseFile err %v", err)
				return err
			}
			// a.基于路径分析包名
			currentPkg, err := deductPkgFromPath(modFileInfo, path)
			if err != nil {
				hlog.CtxWarnf(ctx, "TransverseDirectory deductPkgFromPath err %v", err)
				return err
			}
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
		hlog.CtxWarnf(ctx, "TransverseDirectory Walk err %v", err)
		return nil, err
	}
	return astTransverseInfo, nil
}

func deductPkgFromPath(info *ModFileInfo, filePath string) (string, error) {
	// 获取文件目录
	dir := filepath.Dir(filePath)
	// 将路径分割为部分
	parts := strings.Split(dir, string(filepath.Separator))
	// 组合成实际包名
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid file path")
	}
	// 组合包名
	actualPackageName := info.RootPkg + "/" + strings.Join(parts, "/")
	return actualPackageName, nil
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
		hlog.CtxWarnf(ctx, "TransverseDirectory ReadFile err %v", err)
		return nil, err
	}
	// 解析 go.mod 文件
	modFile, err := modfile.Parse("go.mod", modFileContent, nil)
	if err != nil {
		hlog.CtxWarnf(ctx, "TransverseDirectory Parse err %v", err)
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
