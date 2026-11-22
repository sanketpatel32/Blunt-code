// Command releasesign manages the Ed25519 key that signs release checksums
// for the in-app updater (internal/selfupdate).
//
//	releasesign keygen -out release-signing.key   # prints the public key
//	releasesign sign   -key release-signing.key -in X.zip.sha256 -out X.zip.sha256.sig
//	releasesign verify -pub <base64> -in X.zip.sha256 -sig X.zip.sha256.sig
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"bluntcode/internal/selfupdate"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = keygen(os.Args[2:])
	case "sign":
		err = sign(os.Args[2:])
	case "verify":
		err = verify(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "releasesign:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: releasesign keygen -out FILE | sign -key FILE -in FILE -out FILE | verify [-pub BASE64] -in FILE -sig FILE")
}

func keygen(args []string) error {
	flags := flag.NewFlagSet("keygen", flag.ExitOnError)
	out := flags.String("out", "", "private key file to create")
	_ = flags.Parse(args)
	if *out == "" {
		return errors.New("-out is required")
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(*out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(base64.StdEncoding.EncodeToString(private.Seed()) + "\n"); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	fmt.Println(base64.StdEncoding.EncodeToString(public))
	return nil
}

func sign(args []string) error {
	flags := flag.NewFlagSet("sign", flag.ExitOnError)
	keyPath := flags.String("key", "", "private key file from keygen")
	in := flags.String("in", "", "file to sign (the .sha256 asset)")
	out := flags.String("out", "", "signature file to write")
	_ = flags.Parse(args)
	if *keyPath == "" || *in == "" || *out == "" {
		return errors.New("-key, -in and -out are required")
	}
	raw, err := os.ReadFile(*keyPath)
	if err != nil {
		return err
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return errors.New("private key file is malformed")
	}
	message, err := os.ReadFile(*in)
	if err != nil {
		return err
	}
	signature := ed25519.Sign(ed25519.NewKeyFromSeed(seed), message)
	return os.WriteFile(*out, []byte(base64.StdEncoding.EncodeToString(signature)), 0o644)
}

func verify(args []string) error {
	flags := flag.NewFlagSet("verify", flag.ExitOnError)
	public := flags.String("pub", selfupdate.ReleaseSigningKey, "base64 public key (defaults to the compiled-in key)")
	in := flags.String("in", "", "signed file")
	sig := flags.String("sig", "", "signature file")
	_ = flags.Parse(args)
	if *public == "" || *in == "" || *sig == "" {
		return errors.New("-pub (or a compiled-in key), -in and -sig are required")
	}
	message, err := os.ReadFile(*in)
	if err != nil {
		return err
	}
	signature, err := os.ReadFile(*sig)
	if err != nil {
		return err
	}
	if err := selfupdate.VerifySignature(*public, message, signature); err != nil {
		return err
	}
	fmt.Println("signature OK")
	return nil
}
