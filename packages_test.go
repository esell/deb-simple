package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"testing"
)

var goodOutput = `Package: vim-tiny
Source: vim
Version: 2:7.4.052-1ubuntu3
Architecture: amd64
Maintainer: Ubuntu Developers <ubuntu-devel-discuss@lists.ubuntu.com>
Installed-Size: 931
Depends: vim-common (= 2:7.4.052-1ubuntu3), libacl1 (>= 2.2.51-8), libc6 (>= 2.15), libselinux1 (>= 1.32), libtinfo5
Suggests: indent
Provides: editor
Section: editors
Priority: important
Homepage: http://www.vim.org/
Description: Vi IMproved - enhanced vi editor - compact version
 Vim is an almost compatible version of the UNIX editor Vi.
 .
 Many new features have been added: multi level undo, syntax
 highlighting, command line history, on-line help, filename
 completion, block operations, folding, Unicode support, etc.
 .
 This package contains a minimal version of vim compiled with no
 GUI and a small subset of features in order to keep small the
 package size. This package does not depend on the vim-runtime
 package, but installing it you will get its additional benefits
 (online documentation, plugins, ...).
Original-Maintainer: Debian Vim Maintainers <pkg-vim-maintainers@lists.alioth.debian.org>
`

var goodPkgGzOutput = `Package: vim-tiny
Source: vim
Version: 2:7.4.052-1ubuntu3
Architecture: amd64
Maintainer: Ubuntu Developers <ubuntu-devel-discuss@lists.ubuntu.com>
Installed-Size: 931
Depends: vim-common (= 2:7.4.052-1ubuntu3), libacl1 (>= 2.2.51-8), libc6 (>= 2.15), libselinux1 (>= 1.32), libtinfo5
Suggests: indent
Provides: editor
Section: editors
Priority: important
Homepage: http://www.vim.org/
Description: Vi IMproved - enhanced vi editor - compact version
 Vim is an almost compatible version of the UNIX editor Vi.
 .
 Many new features have been added: multi level undo, syntax
 highlighting, command line history, on-line help, filename
 completion, block operations, folding, Unicode support, etc.
 .
 This package contains a minimal version of vim compiled with no
 GUI and a small subset of features in order to keep small the
 package size. This package does not depend on the vim-runtime
 package, but installing it you will get its additional benefits
 (online documentation, plugins, ...).
Original-Maintainer: Debian Vim Maintainers <pkg-vim-maintainers@lists.alioth.debian.org>
Filename: dists/stable/main/binary-cats/test.deb
Size: 391240
MD5sum: 0ec79417129746ff789fcff0976730c5
SHA1: b2ac976af80f0f50a8336402d5a29c67a2880b9b
SHA256: 9938ec82a8c882ebc2d59b64b0bf2ac01e9cbc5a235be4aa268d4f8484e75eab
`

var goodPkgGzOutputNonDefault = `Package: vim-tiny
Source: vim
Version: 2:7.4.052-1ubuntu3
Architecture: amd64
Maintainer: Ubuntu Developers <ubuntu-devel-discuss@lists.ubuntu.com>
Installed-Size: 931
Depends: vim-common (= 2:7.4.052-1ubuntu3), libacl1 (>= 2.2.51-8), libc6 (>= 2.15), libselinux1 (>= 1.32), libtinfo5
Suggests: indent
Provides: editor
Section: editors
Priority: important
Homepage: http://www.vim.org/
Description: Vi IMproved - enhanced vi editor - compact version
 Vim is an almost compatible version of the UNIX editor Vi.
 .
 Many new features have been added: multi level undo, syntax
 highlighting, command line history, on-line help, filename
 completion, block operations, folding, Unicode support, etc.
 .
 This package contains a minimal version of vim compiled with no
 GUI and a small subset of features in order to keep small the
 package size. This package does not depend on the vim-runtime
 package, but installing it you will get its additional benefits
 (online documentation, plugins, ...).
Original-Maintainer: Debian Vim Maintainers <pkg-vim-maintainers@lists.alioth.debian.org>
Filename: dists/blah/main/binary-cats/test.deb
Size: 391240
MD5sum: 0ec79417129746ff789fcff0976730c5
SHA1: b2ac976af80f0f50a8336402d5a29c67a2880b9b
SHA256: 9938ec82a8c882ebc2d59b64b0bf2ac01e9cbc5a235be4aa268d4f8484e75eab
`

func TestInspectPackage(t *testing.T) {
	parsedControl, err := inspectPackage("samples/vim-tiny_7.4.052-1ubuntu3_amd64.deb")
	if err != nil {
		t.Errorf("inspectPackage() error: %s", err)
	}
	if parsedControl != goodOutput {
		t.Errorf("control file does not match")
	}

	_, err = inspectPackage("thisfileshouldnotexist")
	if err == nil {
		t.Error("inspectPackage() should have failed, it did not")
	}
}

func TestInspectPackageControl(t *testing.T) {
	sampleDeb, err := ioutil.ReadFile("samples/control.tar.gz")
	if err != nil {
		t.Errorf("error opening sample deb file: %s", err)
	}
	var controlBuf bytes.Buffer
	cfReader := bytes.NewReader(sampleDeb)
	io.Copy(&controlBuf, cfReader)
	parsedControl, err := inspectPackageControl(controlBuf)
	if err != nil {
		t.Errorf("error inspecting control file: %s", err)
	}
	if parsedControl != goodOutput {
		t.Errorf("control file does not match")
	}

	var failControlBuf bytes.Buffer
	_, err = inspectPackageControl(failControlBuf)
	if err == nil {
		t.Error("inspectPackageControl() should have failed, it did not")
	}

}

func TestCreatePackagesGz(t *testing.T) {
	pwd, err := os.Getwd()
	if err != nil {
		t.Errorf("Unable to get current working directory: %s", err)
	}
	config := conf{ListenPort: "9666", RootRepoPath: pwd + "/testing", SupportArch: []string{"cats", "dogs"}, DistroNames: []string{"stable"}, Sections: []string{"main", "blah"}, EnableSSL: false}
	// sanity check...
	if config.RootRepoPath != pwd+"/testing" {
		t.Errorf("RootRepoPath is %s, should be %s\n ", config.RootRepoPath, pwd+"/testing")
	}
	// copy sample deb to repo location (assuming it exists)
	origDeb, err := os.Open("samples/vim-tiny_7.4.052-1ubuntu3_amd64.deb")
	if err != nil {
		t.Errorf("error opening up sample deb: %s", err)
	}
	defer origDeb.Close()
	for _, archDir := range config.SupportArch {
		// do not use the built-in createDirs() in case it is broken
		if err := os.MkdirAll(config.RootRepoPath+"/dists/stable/main/binary-"+archDir, 0755); err != nil {
			t.Errorf("error creating directory for %s: %s\n", archDir, err)
		}
		copyDeb, err := os.Create(config.RootRepoPath + "/dists/stable/main/binary-" + archDir + "/test.deb")
		if err != nil {
			t.Errorf("error creating copy of deb: %s", err)
		}
		_, err = io.Copy(copyDeb, origDeb)
		if err != nil {
			t.Errorf("error writing copy of deb: %s", err)
		}
		if err := copyDeb.Close(); err != nil {
			t.Errorf("error saving copy of deb: %s", err)
		}
	}
	if err := createPackagesGz(config, "stable", "main", "cats"); err != nil {
		t.Errorf("error creating packages gzip for cats")
	}
	pkgGzip, err := ioutil.ReadFile(config.RootRepoPath + "/dists/stable/main/binary-cats/Packages.gz")
	if err != nil {
		t.Errorf("error reading Packages.gz: %s", err)
	}
	pkgReader, err := gzip.NewReader(bytes.NewReader(pkgGzip))
	if err != nil {
		t.Errorf("error reading existing Packages.gz: %s", err)
	}
	buf := bytes.NewBuffer(nil)
	io.Copy(buf, pkgReader)
	if goodPkgGzOutput != buf.String() {
		t.Errorf("Packages.gz does not match, returned value is:\n %s \n\n should be:\n %s", buf.String(), goodPkgGzOutput)
	}

	pkgFile, err := ioutil.ReadFile(config.RootRepoPath + "/dists/stable/main/binary-cats/Packages")
	if err != nil {
		t.Errorf("error reading Packages: %s", err)
	}
	if goodPkgGzOutput != string(pkgFile) {
		t.Errorf("Packages does not match, returned value is:\n %s \n\n should be:\n %s", buf.String(), goodPkgGzOutput)
	}

	// cleanup
	if err := os.RemoveAll(config.RootRepoPath); err != nil {
		t.Errorf("error cleaning up after createPackagesGz(): %s", err)
	}

	// create temp file
	tempFile, err := os.Create(pwd + "/tempFile")
	if err != nil {
		t.Fatalf("create %s: %s", pwd+"/tempFile", err)
	}
	defer tempFile.Close()
	config.RootRepoPath = pwd + "/tempFile"
	// Can't make directory named after file
	if err := createPackagesGz(config, "stable", "main", "cats"); err == nil {
		t.Errorf("createPackagesGz() should have failed, it did not")
	}
	// cleanup
	if err := os.RemoveAll(pwd + "/tempFile"); err != nil {
		t.Errorf("error cleaning up after createDirs(): %s", err)
	}
}

func TestCreatePackagesGzNonDefault(t *testing.T) {
	pwd, err := os.Getwd()
	if err != nil {
		t.Errorf("Unable to get current working directory: %s", err)
	}
	config := conf{ListenPort: "9666", RootRepoPath: pwd + "/testing", SupportArch: []string{"cats", "dogs"}, DistroNames: []string{"blah"}, EnableSSL: false}
	// sanity check...
	if config.RootRepoPath != pwd+"/testing" {
		t.Errorf("RootRepoPath is %s, should be %s\n ", config.RootRepoPath, pwd+"/testing")
	}
	// copy sample deb to repo location (assuming it exists)
	origDeb, err := os.Open("samples/vim-tiny_7.4.052-1ubuntu3_amd64.deb")
	if err != nil {
		t.Errorf("error opening up sample deb: %s", err)
	}
	defer origDeb.Close()
	for _, archDir := range config.SupportArch {
		// do not use the built-in createDirs() in case it is broken
		if err := os.MkdirAll(config.RootRepoPath+"/dists/blah/main/binary-"+archDir, 0755); err != nil {
			t.Errorf("error creating directory for %s: %s\n", archDir, err)
		}
		copyDeb, err := os.Create(config.RootRepoPath + "/dists/blah/main/binary-" + archDir + "/test.deb")
		if err != nil {
			t.Errorf("error creating copy of deb: %s", err)
		}
		_, err = io.Copy(copyDeb, origDeb)
		if err != nil {
			t.Errorf("error writing copy of deb: %s", err)
		}
		if err := copyDeb.Close(); err != nil {
			t.Errorf("error saving copy of deb: %s", err)
		}
	}
	if err := createPackagesGz(config, "blah", "main", "cats"); err != nil {
		t.Errorf("error creating packages gzip for cats")
	}
	pkgGzip, err := ioutil.ReadFile(config.RootRepoPath + "/dists/blah/main/binary-cats/Packages.gz")
	if err != nil {
		t.Errorf("error reading Packages.gz: %s", err)
	}
	pkgReader, err := gzip.NewReader(bytes.NewReader(pkgGzip))
	if err != nil {
		t.Errorf("error reading existing Packages.gz: %s", err)
	}
	buf := bytes.NewBuffer(nil)
	io.Copy(buf, pkgReader)
	if goodPkgGzOutputNonDefault != buf.String() {
		t.Errorf("Packages.gz does not match, returned value is: %s", buf.String())
	}

	// cleanup
	if err := os.RemoveAll(config.RootRepoPath); err != nil {
		t.Errorf("error cleaning up after createPackagesGz(): %s", err)
	}

}
