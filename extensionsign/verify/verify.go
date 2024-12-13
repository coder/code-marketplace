package verify

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/xerrors"
)

// curl https://registry.npmjs.org/@vscode/vsce-sign | jq '.versions[."dist-tags".latest].dist.tarball'

type NPMPackage struct {
	DistTags map[string]string `json:"dist-tags"`
	Versions map[string]struct {
		Dist struct {
			Tarball string `json:"tarball"`
		}
	}
}

// go run /home/steven/go/src/github.com/coder/code-marketplace/cmd/marketplace/main.go add --extensions-dir ./extensions -v --key=./extensionsign/testdata/key2.pem --certs=./extensionsign/testdata/cert2.pem --save-sigs https://github.com/VSCodeVim/Vim/releases/download/v1.24.1/vim-1.24.1.vsix
//
// ./node_modules/@vscode/vsce-sign/bin/vsce-sign verify  -v --package ../../extensions/vscodevim/vim/1.24.1/vscodevim.vim-1.24.1.vsix  --signaturearchive ../../extensions/vscodevim/vim/1.24.1/signature.p7s
// ./node_modules/@vscode/vsce-sign/bin/vsce-sign verify --package ./examples/Microsoft.VisualStudio.Services.VSIXPackage --signaturearchive ./examples/Microsoft.VisualStudio.Services.VsixSignature -v
func DownloadVsceSign(ctx context.Context) error {
	cli := http.DefaultClient
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://registry.npmjs.org/@vscode/vsce-sign", nil)
	if err != nil {
		return err
	}

	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return xerrors.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var pkg NPMPackage
	err = json.NewDecoder(resp.Body).Decode(&pkg)
	if err != nil {
		return xerrors.Errorf("decode package: %w", err)
	}

	// If this panics, sorry
	tarURL := pkg.Versions[pkg.DistTags["latest"]].Dist.Tarball
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, tarURL, nil)
	if err != nil {
		return xerrors.Errorf("create tar request: %w", err)
	}

	resp, err = cli.Do(req)
	if err != nil {
		return xerrors.Errorf("do tar request: %w", err)
	}

	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return xerrors.Errorf("create gzip reader: %w", err)
	}

	r := tar.NewReader(gzReader)
	for {
		hdr, err := r.Next()
		if err != nil {
			return err
		}
		fmt.Println(hdr.Name)
	}

	return nil
}
