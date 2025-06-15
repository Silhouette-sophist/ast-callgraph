package main

import (
	"ast-callgraph/service"
	"context"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

func main() {
	ctx := context.Background()
	start := time.Now()
	defer func() {
		hlog.CtxInfof(ctx, "exec cost %.2f", time.Since(start).Seconds())
	}()
	astTransverseInfo, err := service.TransverseDirectory(ctx, &service.AstTransverseParam{
		Directory: "/Users/silhouette/work-practice/ast-callgraph",
	})
	if err != nil {
		hlog.CtxInfof(ctx, "TransverseDirectory err: %v", err)
		return
	}
	hlog.CtxInfof(ctx, "TransverseDirectory success %v", astTransverseInfo)
}
