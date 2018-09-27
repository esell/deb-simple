package main

import (
	"bytes"
	"io"
	"log"
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"
	"golang.org/x/crypto/openpgp/packet"
)

var goodReleaseOutput = `Suite: stable
Codename: stable
Components: main blah
Architectures: cats dogs
Date: Thu, 20 Sep 2018 14:17:21 UTC
MD5Sum:
 40fb9665d0d186102bad50191484910f 1307 main/binary-cats/Packages
 5f05d1302a6a356198b2d2ffffa7933d 820 main/binary-cats/Packages.gz
SHA1:
 fc08372f4853c4d958216399bcdba492cb21d72f 1307 main/binary-cats/Packages
 d352275a13b69f2a67c7c6616a02ee00d7d7d591 820 main/binary-cats/Packages.gz
SHA256:
 ef0a50955545e01dd2dae7ee67e75e59c6be8e2b4f106085528c9386b5dcb62e 1307 main/binary-cats/Packages
 97fe74cd7c19dc0f37726466af800909c9802a468ec1db4528a624ea0901547d 820 main/binary-cats/Packages.gz
`

func TestCreateRelease(t *testing.T) {

	Now = func() time.Time {
		return time.Date(2018, 9, 20, 14, 17, 21, 000000000, time.UTC)
	}

	pwd, err := os.Getwd()
	if err != nil {
		t.Errorf("Unable to get current working directory: %s", err)
	}
	config := conf{ListenPort: "9666", RootRepoPath: pwd + "/testing", SupportArch: []string{"cats", "dogs"}, DistroNames: []string{"stable"}, Sections: []string{"main", "blah"}, EnableSSL: false, EnableSigning: true, PrivateKey: pwd + "/testing/private.key"}

	// do not use the built-in createDirs() in case it is broken
	if err := os.MkdirAll(config.RootRepoPath+"/dists/stable/main/binary-cats", 0755); err != nil {
		t.Errorf("error creating directory: %s\n", err)
	}

	origPackages, err := os.Open("samples/Packages")
	if err != nil {
		t.Errorf("error opening up sample packages: %s", err)
	}
	copyPackages, err := os.Create(config.RootRepoPath + "/dists/stable/main/binary-cats/Packages")
	if err != nil {
		t.Errorf("error creating copy of package file: %s", err)
	}
	defer copyPackages.Close()
	_, err = io.Copy(copyPackages, origPackages)
	if err != nil {
		t.Errorf("error writing copy of package file: %s", err)
	}

	origPackagesGz, err := os.Open("samples/Packages.gz")
	if err != nil {
		t.Errorf("error opening up sample packages: %s", err)
	}
	copyPackagesGz, err := os.Create(config.RootRepoPath + "/dists/stable/main/binary-cats/Packages.gz")
	if err != nil {
		t.Errorf("error creating copy of package file: %s", err)
	}
	defer copyPackagesGz.Close()
	_, err = io.Copy(copyPackagesGz, origPackagesGz)
	if err != nil {
		t.Errorf("error writing copy of package file: %s", err)
	}

	createKeyHandler(pwd+"/testing", "deb-simple test", "blah@blah.com")
	if err := createRelease(config, "stable"); err != nil {
		t.Errorf("error creating Releases file: %s", err)
	}

	releaseFile, err := os.Open(config.RootRepoPath + "/dists/stable/Release")
	if err != nil {
		t.Errorf("error reading Release file: %s", err)
	}
	buf := bytes.NewBuffer(nil)
	io.Copy(buf, releaseFile)
	if goodReleaseOutput != string(buf.String()) {
		t.Errorf("Releases does not match, returned value is:\n %s \n\n should be:\n %s", buf.String(), goodReleaseOutput)
	}

	if err := os.RemoveAll(config.RootRepoPath); err != nil {
		t.Errorf("error cleaning up after createRelease(): %s", err)
	}
}

func TestCreateKey(t *testing.T) {
	pwd, _ := os.Getwd()

	if err := os.MkdirAll("testing", 0755); err != nil {
		t.Errorf("error creating directory: %s\n", err)
	}

	createKeyHandler(pwd+"/testing", "deb-simple Test", "deb-simple@go.go")

	privateKey, err := os.Stat("testing/private.key")
	if os.IsNotExist(err) {
		t.Errorf("Private key was not saved correctly: %s", err)
	}

	if privateKey.Size() < 3000 {
		t.Error("Private key was too small")
	}

	if err := os.Remove("testing/private.key"); err != nil {
		t.Errorf("error cleaning up private key: %s", err)
	}

	publicKey, err := os.Stat("testing/public.key")
	if os.IsNotExist(err) {
		t.Errorf("Public key was not saved correctly: %s", err)
	}

	if publicKey.Size() < 1500 {
		t.Error("Public key was too small")
	}

	if err := os.Remove("testing/public.key"); err != nil {
		t.Errorf("error cleaning up public key: %s", err)
	}
}

func TestSignRelease(t *testing.T) {
	pwd, err := os.Getwd()
	if err != nil {
		t.Errorf("Unable to get current working directory: %s", err)
	}

	createKeyHandler(pwd+"/testing", "deb-simple Test", "deb-simple@go.go")

	config := conf{ListenPort: "9666", RootRepoPath: pwd + "/testing", SupportArch: []string{"cats"}, DistroNames: []string{"stable"}, Sections: []string{"main"}, EnableSSL: false, EnableSigning: true, PrivateKey: pwd + "/testing/private.key"}

	origPackages, err := os.Open("samples/Packages.gz")
	if err != nil {
		t.Errorf("error opening up sample packages: %s", err)
	}
	// do not use the built-in createDirs() in case it is broken
	if err := os.MkdirAll(config.RootRepoPath+"/dists/stable/main/binary-cats", 0755); err != nil {
		t.Errorf("error creating directory: %s\n", err)
	}
	copyPackages, err := os.Create(config.RootRepoPath + "/dists/stable/main/binary-cats/Packages.gz")
	if err != nil {
		t.Errorf("error creating copy of package file: %s", err)
	}
	defer copyPackages.Close()
	_, err = io.Copy(copyPackages, origPackages)
	if err != nil {
		t.Errorf("error writing copy of package file: %s", err)
	}

	if err := createRelease(config, "stable"); err != nil {
		t.Errorf("error creating Releases file: %s", err)
	}

	gpgSig, err := os.Stat(config.RootRepoPath + "/dists/stable/Release.gpg")
	if os.IsNotExist(err) {
		t.Errorf("GPG Signature was not saved correctly: %s", err)
	}

	if gpgSig.Size() < 400 {
		t.Error("GPG Signature was too small")
	}

	signedRelease, err := os.Stat(config.RootRepoPath + "/dists/stable/InRelease")
	if os.IsNotExist(err) {
		t.Errorf("Signed Release file was not saved correctly: %s", err)
	}

	if signedRelease.Size() < 800 {
		t.Error("Signed release was too small")
	}

	signature, _ := os.Open(config.RootRepoPath + "/dists/stable/Release.gpg")
	message, _ := os.Open(config.RootRepoPath + "/dists/stable/Release")
	block, _ := armor.Decode(signature)
	reader := packet.NewReader(block.Body)
	pkt, _ := reader.Next()
	sig, _ := pkt.(*packet.Signature)
	hash := sig.Hash.New()
	io.Copy(hash, message)

	entity := createEntityFromPublicKey(pwd + "/testing/public.key")

	err = entity.PrimaryKey.VerifySignature(hash, sig)
	if err != nil {
		t.Errorf("Could not verify Release file: %s", err)
	}

	if err := os.RemoveAll(config.RootRepoPath); err != nil {
		t.Errorf("error cleaning up after createRelease(): %s", err)
	}
}

func createEntityFromPublicKey(publicKeyPath string) *openpgp.Entity {

	publicKeyData, err := os.Open(publicKeyPath)
	if err != nil {
		log.Fatalf("Error opening public key file %s: %s", publicKeyPath, err)
	}
	defer publicKeyData.Close()

	block, err := armor.Decode(publicKeyData)

	if err != nil {
		log.Fatalf("Error decoding public key data: %s", err)
	}

	if block.Type != openpgp.PublicKeyType {
		log.Fatalf("Invalid public key type %s", block.Type)
	}

	reader := packet.NewReader(block.Body)
	pkt, err := reader.Next()

	if err != nil {
		log.Fatalf("Error reading public key data: %s", err)
	}

	publicKey, ok := pkt.(*packet.PublicKey)
	if !ok {
		log.Fatalf("Error parsing public key")
	}

	e := openpgp.Entity{
		PrimaryKey: publicKey,
	}

	return &e
}
