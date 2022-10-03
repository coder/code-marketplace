variable "VERSION" {}

group "default" {
  targets = ["code-marketplace"]
}

target "code-marketplace" {
  dockerfile = "./Dockerfile"
  tags = [
    "ghcr.io/coder/code-marketplace:${VERSION}",
  ]
  platforms = ["linux/amd64", "linux/arm64"]
}
