package main

import "embed"

// frontend/dist 由 make build-dist 产出；仓库内置 .gitkeep 占位保证未构建时也可编译。
//
//go:embed all:frontend/dist
var distFS embed.FS
