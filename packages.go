package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"errors"

	"github.com/blakesmith/ar"
	lzma "github.com/xi2/xz"
)

type Compression int

const (
	LZMA Compression = iota
	GZIP
)

func inspectPackage(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", fmt.Errorf("error opening package file %s: %s", filename, err)
	}

	if *verbose {
		log.Printf("Inpecting package file \"%s\"", filename)
	}

	arReader := ar.NewReader(f)
	defer f.Close()
	var controlBuf bytes.Buffer

	for {
		header, err := arReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return "", fmt.Errorf("error in inspectPackage loop: %s", err)
		}

		if strings.Contains(header.Name, "control.tar") {
			var compression Compression
			if strings.TrimRight(header.Name, "/") == "control.tar.gz" {
				compression = GZIP
			} else if strings.TrimRight(header.Name, "/") == "control.tar.xz" {
				compression = LZMA
			} else {
				log.Println("\t No control file found")
				err := errors.New("No control file found")
				return "", err
			}

			io.Copy(&controlBuf, arReader)
			if *verbose {
				log.Println("\t Found a package control file")
			}
			return inspectPackageControl(compression, controlBuf)
		}

	}
	return "", nil
}

func inspectPackageControl(compression Compression, filename bytes.Buffer) (string, error) {
	var tarReader *tar.Reader
	var err error

	switch compression {
	case GZIP:
		var compFile *gzip.Reader
		compFile, err = gzip.NewReader(bytes.NewReader(filename.Bytes()))
		tarReader = tar.NewReader(compFile)
		if *verbose {
			log.Println("\t GZIP Control file found")
		}
		break
	case LZMA:
		var compFile *lzma.Reader
		compFile, err = lzma.NewReader(bytes.NewReader(filename.Bytes()), lzma.DefaultDictMax)
		tarReader = tar.NewReader(compFile)
		if *verbose {
			log.Println("\t LZMA Control file found")
		}
		break
	}

	if err != nil {
		return "", fmt.Errorf("error creating gzip/lzma reader: %s", err)
	}

	var controlBuf bytes.Buffer
	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return "", fmt.Errorf("failed to inspect package: %s", err)
		}

		name := header.Name

		switch header.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
			switch name {
			case "control", "./control":
				io.Copy(&controlBuf, tarReader)
				return strings.TrimRight(controlBuf.String(), "\n") + "\n", nil
			}
		default:
			log.Printf(
				"Unable to figure out type : %c in file %s\n",
				header.Typeflag, name,
			)
		}
	}
	return "", nil
}

func createPackagesGz(config conf, distro, section, arch string) error {

	if *verbose {
		log.Printf("Rebuilding Packages.gz file for %s %s %s", distro, section, arch)
	}

	packageFile, err := os.Create(filepath.Join(config.ArchPath(distro, section, arch), "Packages"))
	if err != nil {
		return fmt.Errorf("failed to create Packages: %s", err)
	}
	packageGzFile, err := os.Create(filepath.Join(config.ArchPath(distro, section, arch), "Packages.gz"))
	if err != nil {
		return fmt.Errorf("failed to create packages.gz: %s", err)
	}
	defer packageFile.Close()
	defer packageGzFile.Close()
	gzOut := gzip.NewWriter(packageGzFile)
	defer gzOut.Close()

	writer := io.MultiWriter(packageFile, gzOut)

	// loop through each directory
	// run inspectPackage
	dirList, err := ioutil.ReadDir(config.ArchPath(distro, section, arch))
	if err != nil {
		return fmt.Errorf("scanning: %s: %s", config.ArchPath(distro, section, arch), err)
	}
	for i, debFile := range dirList {
		if strings.HasSuffix(debFile.Name(), "deb") {
			var packBuf bytes.Buffer
			debPath := filepath.Join(config.ArchPath(distro, section, arch), debFile.Name())
			tempCtlData, err := inspectPackage(debPath)
			if err != nil {
				return err
			}
			packBuf.WriteString(tempCtlData)
			dir := filepath.ToSlash(filepath.Join("dists", distro, section, "binary-"+arch, debFile.Name()))
			fmt.Fprintf(&packBuf, "Filename: %s\n", dir)
			fmt.Fprintf(&packBuf, "Size: %d\n", debFile.Size())
			f, err := os.Open(debPath)
			if err != nil {
				log.Println("error opening deb file: ", err)
			}
			defer f.Close()

			var (
				md5hash    = md5.New()
				sha1hash   = sha1.New()
				sha256hash = sha256.New()
			)
			_, err = io.Copy(io.MultiWriter(md5hash, sha1hash, sha256hash), f)
			if err != nil {
				return fmt.Errorf("Error hashing file for Packages file: %s", err)
			}
			fmt.Fprintf(&packBuf, "MD5sum: %s\n",
				hex.EncodeToString(md5hash.Sum(nil)))
			fmt.Fprintf(&packBuf, "SHA1: %s\n",
				hex.EncodeToString(sha1hash.Sum(nil)))
			fmt.Fprintf(&packBuf, "SHA256: %s\n",
				hex.EncodeToString(sha256hash.Sum(nil)))
			if i != (len(dirList) - 1) {
				packBuf.WriteString("\n\n")
			}
			writer.Write(packBuf.Bytes())
			f = nil
		}
	}

	return nil
}
