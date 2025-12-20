# 构建前端
set -e
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR/web"
npm run generate

# 同步静态文件到后端 public
rsync -a --delete .output/public/ "$ROOT_DIR/public/"