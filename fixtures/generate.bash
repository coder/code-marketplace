#!/usr/bin/env bash
# Generate a ./extensions directory with names based on text files containing
# randomly generated words.

set -Eeuo pipefail

dir=$(dirname "$0")

# Pretty arbitrary but try to ensure we have a variety of extension kinds.
kinds=("workspace", "ui,web", "workspace,web", "ui")

while read -r publisher ; do
  i=0
  while read -r name ; do
    kind=${kinds[$i]-workspace}
    ((++i))
    while read -r version ; do
      dest="./extensions/$publisher/$name/$version"
      mkdir -p "$dest/extension/images"
      cat<<EOF > "$dest/extension.vsixmanifest"
<?xml version="1.0" encoding="utf-8"?>
  <PackageManifest Version="2.0.0" xmlns="http://schemas.microsoft.com/developer/vsx-schema/2011" xmlns:d="http://schemas.microsoft.com/developer/vsx-schema-design/2011">
    <Metadata>
      <Identity Language="en-US" Id="$name" Version="$version" Publisher="$publisher" />
      <DisplayName>$name</DisplayName>
      <Description xml:space="preserve">Mock extension for Visual Studio Code</Description>
      <Tags>$name,mock,tag1</Tags>
      <Categories>category1,category2</Categories>
      <GalleryFlags>Public</GalleryFlags>

      <Properties>
        <Property Id="Microsoft.VisualStudio.Code.Engine" Value="^1.57.0" />
        <Property Id="Microsoft.VisualStudio.Code.ExtensionDependencies" Value="" />
        <Property Id="Microsoft.VisualStudio.Code.ExtensionPack" Value="" />
        <Property Id="Microsoft.VisualStudio.Code.ExtensionKind" Value="$kind" />
        <Property Id="Microsoft.VisualStudio.Code.LocalizedLanguages" Value="" />

        <Property Id="Microsoft.VisualStudio.Services.Links.Source" Value="https://github.com/coder/code-marketplace.git" />
        <Property Id="Microsoft.VisualStudio.Services.Links.Getstarted" Value="https://github.com/coder/code-marketplace.git" />
        <Property Id="Microsoft.VisualStudio.Services.Links.GitHub" Value="https://github.com/coder/code-marketplace.git" />
        <Property Id="Microsoft.VisualStudio.Services.Links.Support" Value="https://github.com/coder/code-marketplace/issues" />
        <Property Id="Microsoft.VisualStudio.Services.Links.Learn" Value="https://github.com/coder/code-marketplace" />
        <Property Id="Microsoft.VisualStudio.Services.Branding.Color" Value="#e3f4ff" />
        <Property Id="Microsoft.VisualStudio.Services.Branding.Theme" Value="light" />
        <Property Id="Microsoft.VisualStudio.Services.GitHubFlavoredMarkdown" Value="true" />

        <Property Id="Microsoft.VisualStudio.Services.CustomerQnALink" Value="https://github.com/coder/code-marketplace" />
      </Properties>
      <License>extension/LICENSE.txt</License>
      <Icon>extension/images/icon.png</Icon>
    </Metadata>
    <Installation>
      <InstallationTarget Id="Microsoft.VisualStudio.Code"/>
    </Installation>
    <Dependencies/>
    <Assets>
      <Asset Type="Microsoft.VisualStudio.Code.Manifest" Path="extension/package.json" Addressable="true" />
      <Asset Type="Microsoft.VisualStudio.Services.Content.Details" Path="extension/README.md" Addressable="true" />
      <Asset Type="Microsoft.VisualStudio.Services.Content.Changelog" Path="extension/CHANGELOG.md" Addressable="true" />
      <Asset Type="Microsoft.VisualStudio.Services.Content.License" Path="extension/LICENSE.txt" Addressable="true" />
      <Asset Type="Microsoft.VisualStudio.Services.Icons.Default" Path="extension/images/icon.png" Addressable="true" />
    </Assets>
  </PackageManifest>
EOF
      cat<<EOF > "$dest/extension/package.json"
{
  "name": "$name",
  "displayName": "$name",
  "description": "Mock extension for Visual Studio Code",
  "version": "$version",
  "publisher": "$publisher",
  "author": {
    "name": "Coder"
  },
  "license": "MIT",
  "homepage": "https://github.com/coder/code-marketplace",
  "repository": {
    "type": "git",
    "url": "https://github.com/coder/code-marketplace"
  },
  "bugs": {
    "url": "https://github.com/coder/code-marketplace/issues"
  },
  "qna": "https://github.com/coder/code-marketplace",
  "icon": "icon.png",
  "engines": {
    "vscode": "^1.57.0"
  },
  "keywords": [
    "$name",
    "mock",
    "coder",
  ],
  "categories": [
    "Category1",
    "Category2"
  ],
  "activationEvents": [
    "onStartupFinished"
  ],
  "main": "./extension",
  "browser": "./extension.browser.js"
}
EOF
      cat<<EOF > "$dest/extension/extension.js"
const vscode = require("vscode");
function activate(context) {
  vscode.window.showInformationMessage("mock extension $publisher.$name-$version loaded");
}
exports.activate = activate;
EOF
      cp "$dest/extension/extension.js" "$dest/extension/extension.browser.js"
      cat<<EOF > "$dest/extension/LICENSE.txt"
mock license
EOF
      cat<<EOF > "$dest/extension/README.md"
mock readme
EOF
      cat<<EOF > "$dest/extension/CHANGELOG.md"
mock changelog
EOF
      cp "$dir/icon.png" "$dest/extension/images/icon.png"
      pushd "$dest" >/dev/null
      rm "$publisher.$name-$version.vsix"
      zip -r "$publisher.$name-$version.vsix" * -q
      popd >/dev/null
    done < "$dir/versions"
  done < "$dir/names"
done < "$dir/publishers"
