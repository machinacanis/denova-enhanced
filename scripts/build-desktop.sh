#!/bin/bash
# build-desktop.sh - 构建 Denova 桌面应用 (Wails v3)
#
# 依赖:
#   - Go >= 1.26.5
#   - Node.js >= 20 + pnpm
#   - Linux: webkitgtk-6.0 (pacman -S webkitgtk-6.0)
#   - macOS: Xcode Command Line Tools
#   - Windows: WebView2 Runtime (预装于 Win11)
#
# 用法:
#   ./scripts/build-desktop.sh          # 构建桌面二进制（dist/ 为自包含发布包）
#   ./scripts/build-desktop.sh --dev    # 开发模式运行

set -e

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${ROOT_DIR}"

MODE="build"
if [ "$1" = "--dev" ]; then
    MODE="dev"
fi

echo "==> Denova 桌面应用构建"
echo "  模式: ${MODE}"
echo ""

# 1. 检查 Go 依赖
echo "==> 拉取 Go 依赖"
go mod tidy

# 2. 构建前端
echo "==> 构建前端资源"
if [ ! -d "web/node_modules" ]; then
    echo "  安装前端依赖..."
    (cd web && pnpm install)
fi
(cd web && pnpm run build)

# 3. 构建/运行桌面应用
if [ "${MODE}" = "dev" ]; then
    echo "==> 开发模式启动桌面应用"
    echo "  按 Ctrl+C 停止"
    echo ""
    exec go run ./cmd/denova-desktop --dev-mode
else
    echo "==> 编译桌面二进制"
    # GOEXE 在 Windows 上为 ".exe"，其他平台为空，避免手动重命名
    OUTPUT="dist/denova-desktop$(go env GOEXE)"
    mkdir -p dist

    # 注入版本信息
    VERSION=$(git describe --tags --always 2>/dev/null || echo "dev")
    BUILD_TIME=$(date -u '+%Y-%m-%dT%H:%M:%SZ')

    go build \
        -ldflags "-X denova/internal/buildinfo.Version=${VERSION} -X denova/internal/buildinfo.BuildTime=${BUILD_TIME}" \
        -o "${OUTPUT}" \
        ./cmd/denova-desktop

    # 打包随包资源，使 dist/ 成为自包含发布包（与 build.sh / build-github-release.sh
    # 的“资源与二进制同级”约定一致）。运行时经 exeDir/web/dist、exeDir/skills
    # 解析（见 cmd/denova-desktop），整个目录可拷贝到任意位置运行。
    echo "==> 打包前端产物与 skills 到 dist/"
    rm -rf dist/web dist/skills
    mkdir -p dist/web
    cp -r web/dist dist/web/dist
    cp -r skills dist/skills
    if [ -f config.toml ]; then
        cp config.toml dist/config.toml
    fi

    echo ""
    echo "  构建完成: ${OUTPUT}"
    echo "  发布包: dist/ (含 denova-desktop、web/dist、skills，可整体分发)"
    echo "  运行: ${OUTPUT}"
fi
