variable "GITHUB_REF_NAME" {}

group "default" {
  targets = ["code-marketplace"]
}

target "code-marketplace" {
  dockerfile = "./Dockerfile"
  tags = [
    "ghcr.io/coder/code-marketplace:${GITHUB_REF_NAME}",
  ]
  platforms = ["linux/amd64", "linux/arm64"]
}
