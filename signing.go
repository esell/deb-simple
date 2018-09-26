package main

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"
	"golang.org/x/crypto/openpgp/clearsign"
	"golang.org/x/crypto/openpgp/packet"
)

// createRelease scans for Packages files and builds a Release file summary, then signs it with a key.
// Both Packages and Packages.gz files are included and hashed.
func createRelease(config conf, distro string) error {

	if *verbose {
		log.Printf("Creating release file for \"%s\"", distro)
	}

	workingDirectory := filepath.Join(config.RootRepoPath, "dists", distro)

	outfile, err := os.Create(filepath.Join(workingDirectory, "Release"))
	if err != nil {
		return fmt.Errorf("failed to create Release: %s", err)
	}
	defer outfile.Close()

	currentTime := Now().UTC()
	fmt.Fprintf(outfile, "Suite: %s\n", distro)
	fmt.Fprintf(outfile, "Codename: %s\n", distro)
	fmt.Fprintf(outfile, "Components: %s\n", strings.Join(config.Sections, " "))
	fmt.Fprintf(outfile, "Architectures: %s\n", strings.Join(config.SupportArch, " "))
	fmt.Fprintf(outfile, "Date: %s\n", currentTime.Format("Mon, 02 Jan 2006 15:04:05 UTC"))

	var md5Sums strings.Builder
	var sha1Sums strings.Builder
	var sha256Sums strings.Builder

	err = filepath.Walk(workingDirectory, func(path string, file os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(path, "Packages.gz") || strings.HasSuffix(path, "Packages") {

			var (
				md5hash    = md5.New()
				sha1hash   = sha1.New()
				sha256hash = sha256.New()
			)

			relPath, _ := filepath.Rel(workingDirectory, path)
			spath := filepath.ToSlash(relPath)
			f, err := os.Open(path)

			if err != nil {
				log.Println("Error opening the Packages file for reading", err)
			}

			if _, err = io.Copy(io.MultiWriter(md5hash, sha1hash, sha256hash), f); err != nil {
				return fmt.Errorf("Error hashing file for Release list: %s", err)
			}
			fmt.Fprintf(&md5Sums, " %s %d %s\n",
				hex.EncodeToString(md5hash.Sum(nil)),
				file.Size(), spath)
			fmt.Fprintf(&sha1Sums, " %s %d %s\n",
				hex.EncodeToString(sha1hash.Sum(nil)),
				file.Size(), spath)
			fmt.Fprintf(&sha256Sums, " %s %d %s\n",
				hex.EncodeToString(sha256hash.Sum(nil)),
				file.Size(), spath)

			f = nil
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("Error scanning for Packages files: %s", err)
	}

	outfile.WriteString("MD5Sum:\n")
	outfile.WriteString(md5Sums.String())
	outfile.WriteString("SHA1:\n")
	outfile.WriteString(sha1Sums.String())
	outfile.WriteString("SHA256:\n")
	outfile.WriteString(sha256Sums.String())

	if err = signRelease(config, outfile.Name()); err != nil {
		return fmt.Errorf("Error signing Release file: %s", err)
	}

	return nil
}

// signRelease takes the path to an existing Release file, and signs it with the configured private key.
// Both Release.gpg (detached signature) and InRelease (inline signature) will be generated, in order to
// ensure maximum compatibility
func signRelease(config conf, filename string) error {

	if *verbose {
		log.Printf("Signing release file \"%s\"", filename)
	}

	entity := createEntityFromPrivateKey(config.PrivateKey)

	workingDirectory := filepath.Dir(filename)

	releaseFile, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("Error opening Release file (%s) for writing: %s", filename, err)
	}

	releaseGpg, err := os.Create(filepath.Join(workingDirectory, "Release.gpg"))
	if err != nil {
		return fmt.Errorf("Error creating Release.gpg file for writing: %s", err)
	}
	defer releaseGpg.Close()

	err = openpgp.ArmoredDetachSign(releaseGpg, entity, releaseFile, nil)
	if err != nil {
		return fmt.Errorf("Error writing signature to Release.gpg file: %s", err)
	}

	releaseFile.Seek(0, 0)

	inlineRelease, err := os.Create(filepath.Join(workingDirectory, "InRelease"))
	if err != nil {
		return fmt.Errorf("Error creating InRelease file for writing: %s", err)
	}
	defer inlineRelease.Close()

	writer, err := clearsign.Encode(inlineRelease, entity.PrivateKey, nil)
	if err != nil {
		return fmt.Errorf("Error signing InRelease file : %s", err)
	}

	io.Copy(writer, releaseFile)
	writer.Close()

	return nil
}

// createKeyPair generates a new OpenPGP Entity with the provided name, comment and email.
// The keys are returned as ASCII Armor encoded strings, ready to write to files.
func createKeyPair(name, comment, email string) (*openpgp.Entity, string, string) {

	entity, err := openpgp.NewEntity(name, comment, email, nil)
	if err != nil {
		log.Fatalf("Error creating openpgp entity: %s", err)
	}

	serializedPrivateEntity := bytes.NewBuffer(nil)
	entity.SerializePrivate(serializedPrivateEntity, nil)
	serializedEntity := bytes.NewBuffer(nil)
	entity.Serialize(serializedEntity)

	buf := bytes.NewBuffer(nil)
	headers := map[string]string{"Version": "GnuPG v1"}
	w, err := armor.Encode(buf, openpgp.PublicKeyType, headers)
	if err != nil {
		log.Fatal(err)
	}

	_, err = w.Write(serializedEntity.Bytes())
	if err != nil {
		log.Fatalf("Error encoding public key: %s", err)
	}
	w.Close()
	publicKey := buf.String()

	buf = bytes.NewBuffer(nil)
	w, err = armor.Encode(buf, openpgp.PrivateKeyType, headers)
	if err != nil {
		log.Fatal(err)
	}
	_, err = w.Write(serializedPrivateEntity.Bytes())
	if err != nil {
		log.Fatalf("Error encoding private key: %s", err)
	}
	w.Close()
	privateKey := buf.String()

	return entity, publicKey, privateKey
}

// createEntityFromPrivateKey creates a new OpenPGP Entity objects from the provided private key path.
// The key should be in ASCII Armour format.
// The returned entity can be used to sign files - the public key / identity is not needed.
func createEntityFromPrivateKey(privateKeyPath string) *openpgp.Entity {

	privateKeyData, err := os.Open(privateKeyPath)

	if err != nil {
		log.Fatalf("Error opening private key file: %s", err)
	}
	defer privateKeyData.Close()

	block, err := armor.Decode(privateKeyData)

	if err != nil {
		log.Fatalf("Error decoding private key data: %s", err)
	}

	if block.Type != openpgp.PrivateKeyType {
		log.Fatalf("Invalid private key type %s", block.Type)
	}

	reader := packet.NewReader(block.Body)
	pkt, err := reader.Next()

	if err != nil {
		log.Fatalf("Error reading private key data: %s", err)
	}

	privateKey, ok := pkt.(*packet.PrivateKey)
	if !ok {
		log.Fatalf("Error parsing private key")
	}

	e := openpgp.Entity{
		PrivateKey: privateKey,
	}

	return &e
}

// createKeyHandler generates a new public and private key pair, and writes them out to workingDirectory.
func createKeyHandler(workingDirectory, name, email string) {

	_, publicKey, privateKey := createKeyPair(name, "Generated by deb-simple", email)

	pubFile, err := os.Create(filepath.Join(workingDirectory, "public.key"))
	if err != nil {
		log.Fatalf("Could not open public key file for writing: %s", err)
	}
	defer pubFile.Close()

	pubFile.WriteString(publicKey)

	priFile, err := os.Create(filepath.Join(workingDirectory, "private.key"))
	if err != nil {
		log.Fatalf("Could not open private key file for writing: %s", err)
	}
	defer priFile.Close()

	priFile.WriteString(privateKey)
}
