if [ -z "$VERSION" ]; then
  echo "warning: VERSION not set, using latest"
  VERSION=latest
fi

docker buildx create --name lark-leave-helper --driver docker-container --bootstrap --use
docker buildx build --platform linux/amd64,linux/arm64 -t "chenhuanjie/lark-leave-helper:${VERSION}" . --push
docker buildx rm lark-leave-helper
