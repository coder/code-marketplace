package extensionsign

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

func GenerateKey() (ed25519.PrivateKey, error) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return private, nil
}

// To generate a new self signed certificate using openssl:
// openssl req -x509 -newkey Ed25519 -keyout key.pem -out cert.pem -sha256 -days 3650 -nodes -subj "/C=XX/ST=StateName/L=CityName/O=CompanyName/OU=CompanySectionName/CN=CommonNameOrHostname"
// openssl req -x509 -newkey rsa:4096 -keyout key2.pem -out cert2.pem -sha256 -days 3650 -nodes -subj "/C=XX/ST=StateName/L=CityName/O=CompanyName/OU=CompanySectionName/CN=CommonNameOrHostname"

func LoadCertificatesFromDisk(ctx context.Context, logger slog.Logger, files []string) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	for _, file := range files {
		certData, err := os.ReadFile(file)
		if err != nil {
			return nil, xerrors.Errorf("read cert file %q: %w", file, err)
		}

		for {
			block, rest := pem.Decode(certData)

			crt, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, xerrors.Errorf("load certificate %q: %w", file, err)
			}
			logger.Info(ctx, "Loaded certificate",
				slog.F("file", file),
				slog.F("subject", crt.Subject.CommonName),
			)
			certs = append(certs, crt)

			if len(rest) == 0 {
				break
			}
			certData = rest
		}

	}
	return certs, nil
}

// LoadKey takes in a PEM encoded secret
func LoadKey(secret []byte) (crypto.Signer, error) {
	data, rest := pem.Decode(secret)
	if len(rest) > 0 {
		return nil, xerrors.Errorf("extra data after PEM block")
	}

	sec, err := x509.ParsePKCS8PrivateKey(data.Bytes)
	if err != nil {
		_, err2 := x509.ParsePKCS1PrivateKey(secret)
		fmt.Println(err2)
		return nil, err
	}

	signer, ok := sec.(crypto.Signer)
	if !ok {
		return nil, xerrors.Errorf("%T is not a crypto.Signer and is not supported", sec)
	}

	return signer, nil
}
