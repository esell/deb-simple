package main

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/boltdb/bolt"
	"github.com/fsnotify/fsnotify"
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

func TestCreateDirs(t *testing.T) {
	pwd, err := os.Getwd()
	if err != nil {
		t.Errorf("Unable to get current working directory: %s", err)
	}
	config := conf{ListenPort: "9666", RootRepoPath: pwd + "/testing", SupportArch: []string{"cats", "dogs"}, DistroNames: []string{"stable"}, Sections: []string{"main", "blah"}, EnableSSL: false}
	// sanity check...
	if config.RootRepoPath != pwd+"/testing" {
		t.Errorf("RootRepoPath is %s, should be %s\n ", config.RootRepoPath, pwd+"/testing")
	}
	mywatcher, err = fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("error creating fswatcher: ", err)
	}
	t.Log("creating temp dirs in ", config.RootRepoPath)
	if err := createDirs(config); err != nil {
		t.Errorf("createDirs() failed ")
	}
	for _, distName := range config.DistroNames {
		for _, section := range config.Sections {
			for _, archDir := range config.SupportArch {
				if _, err := os.Stat(config.RootRepoPath + "/dists/" + distName + "/" + section + "/binary-" + archDir); err != nil {
					if os.IsNotExist(err) {
						t.Errorf("Directory for %s does not exist", archDir)
					}
				}
			}
		}
	}

	// cleanup
	if err := os.RemoveAll(config.RootRepoPath); err != nil {
		t.Errorf("error cleaning up after createDirs(): %s", err)
	}

	// create temp file
	tempFile, err := os.Create(pwd + "/tempFile")
	if err != nil {
		t.Fatalf("create %s: %s", pwd+"/tempFile", err)
	}
	defer tempFile.Close()
	config.RootRepoPath = pwd + "/tempFile"
	// Can't make directory named after file.
	if err := createDirs(config); err == nil {
		t.Errorf("createDirs() should have failed but did not")
	}
	// cleanup
	if err := os.RemoveAll(pwd + "/tempFile"); err != nil {
		t.Errorf("error cleaning up after createDirs(): %s", err)
	}

}

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
	if goodPkgGzOutput != string(buf.Bytes()) {
		t.Errorf("Packages.gz does not match, returned value is:\n %s \n\n should be:\n %s", string(buf.Bytes()), goodPkgGzOutput)
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
	if goodPkgGzOutputNonDefault != string(buf.Bytes()) {
		t.Errorf("Packages.gz does not match, returned value is: %s", string(buf.Bytes()))
	}

	// cleanup
	if err := os.RemoveAll(config.RootRepoPath); err != nil {
		t.Errorf("error cleaning up after createPackagesGz(): %s", err)
	}

}

func TestUploadHandler(t *testing.T) {
	pwd, err := os.Getwd()
	if err != nil {
		t.Errorf("Unable to get current working directory: %s", err)
	}
	config := conf{ListenPort: "9666", RootRepoPath: pwd + "/testing", SupportArch: []string{"cats", "dogs"}, DistroNames: []string{"stable"}, Sections: []string{"main", "blah"}, EnableSSL: false}
	// sanity check...
	if config.RootRepoPath != pwd+"/testing" {
		t.Errorf("RootRepoPath is %s, should be %s\n ", config.RootRepoPath, pwd+"/testing")
	}
	// create temp db
	db, err := bolt.Open(tempfile(), 0666, nil)
	if err != nil {
		t.Fatalf("error creating tempdb: %s", err)
	}
	defer db.Close()
	uploadHandle := uploadHandler(config, db)
	// GET
	req, _ := http.NewRequest("GET", "", nil)
	w := httptest.NewRecorder()
	uploadHandle.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("uploadHandler GET returned %v, should be %v", w.Code, http.StatusMethodNotAllowed)
	}

	// POST
	// create "all" arch as it's the default
	if err := os.MkdirAll(config.RootRepoPath+"/dists/stable/main/binary-all", 0755); err != nil {
		t.Errorf("error creating directory for POST testing: %s", err)
	}
	sampleDeb, err := os.Open("samples/vim-tiny_7.4.052-1ubuntu3_amd64.deb")
	if err != nil {
		t.Errorf("error opening sample deb file: %s", err)
	}
	defer sampleDeb.Close()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "vim-tiny_7.4.052-1ubuntu3_amd64.deb")
	if err != nil {
		t.Errorf("error FormFile: %s", err)
	}
	_, err = io.Copy(part, sampleDeb)
	if err != nil {
		t.Errorf("error copying sampleDeb to FormFile: %s", err)
	}
	if err := writer.Close(); err != nil {
		t.Errorf("error closing form writer: %s", err)
	}
	req, _ = http.NewRequest("POST", "", body)
	req.Header.Add("Content-Type", writer.FormDataContentType())
	w = httptest.NewRecorder()
	uploadHandle.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("uploadHandler POST returned %v, should be %v", w.Code, http.StatusOK)
	}
	// verify uploaded file matches sample
	uploadFile, _ := ioutil.ReadFile(config.RootRepoPath + "/dists/stable/main/binary-all/vim-tiny_7.4.052-1ubuntu3_amd64.deb")
	uploadmd5hash := md5.New()
	uploadmd5hash.Write(uploadFile)
	uploadFilemd5 := hex.EncodeToString(uploadmd5hash.Sum(nil))

	sampleFile, _ := ioutil.ReadFile("samples/vim-tiny_7.4.052-1ubuntu3_amd64.deb")
	samplemd5hash := md5.New()
	samplemd5hash.Write(sampleFile)
	sampleFilemd5 := hex.EncodeToString(samplemd5hash.Sum(nil))
	if uploadFilemd5 != sampleFilemd5 {
		t.Errorf("uploaded file MD5 is %s, should be %s", uploadFilemd5, sampleFilemd5)
	}

	// cleanup
	if err := os.RemoveAll(config.RootRepoPath); err != nil {
		t.Errorf("error cleaning up after uploadHandler(): %s", err)
	}

	// create temp file
	tempFile, err := os.Create(pwd + "/tempFile")
	if err != nil {
		t.Fatalf("create %s: %s", pwd+"/tempFile", err)
	}
	defer tempFile.Close()
	config.RootRepoPath = pwd + "/tempFile"
	// Can't make directory named after file
	uploadHandle = uploadHandler(config, db)
	failBody := &bytes.Buffer{}
	failWriter := multipart.NewWriter(failBody)
	failPart, err := failWriter.CreateFormFile("file", "vim-tiny_7.4.052-1ubuntu3_amd64.deb")
	if err != nil {
		t.Errorf("error FormFile: %s", err)
	}
	_, err = io.Copy(failPart, sampleDeb)
	if err != nil {
		t.Errorf("error copying sampleDeb to FormFile: %s", err)
	}
	if err := failWriter.Close(); err != nil {
		t.Errorf("error closing form writer: %s", err)
	}
	req, _ = http.NewRequest("POST", "", failBody)
	req.Header.Add("Content-Type", failWriter.FormDataContentType())
	w = httptest.NewRecorder()
	uploadHandle.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("uploadHandler POST returned %v, should be %v", w.Code, http.StatusInternalServerError)
	}
	// cleanup
	if err := os.RemoveAll(pwd + "/tempFile"); err != nil {
		t.Errorf("error cleaning up after createDirs(): %s", err)
	}

	// API key tests
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("APIkeys"))
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("error creating db bucket: %s", err)
	}
	config.EnableAPIKeys = true
	uploadHandle = uploadHandler(config, db)
	// create "all" arch as it's the default
	if err := os.MkdirAll(config.RootRepoPath+"/dists/stable/main/binary-all", 0755); err != nil {
		t.Errorf("error creating directory for POST testing: %s", err)
	}

	sampleDeb, err = os.Open("samples/vim-tiny_7.4.052-1ubuntu3_amd64.deb")
	if err != nil {
		t.Errorf("error opening sample deb file: %s", err)
	}
	defer sampleDeb.Close()

	body = &bytes.Buffer{}
	writer = multipart.NewWriter(body)
	part, err = writer.CreateFormFile("file", "vim-tiny_7.4.052-1ubuntu3_amd64.deb")
	if err != nil {
		t.Errorf("error FormFile: %s", err)
	}
	_, err = io.Copy(part, sampleDeb)
	if err != nil {
		t.Errorf("error copying sampleDeb to FormFile: %s", err)
	}
	if err := writer.Close(); err != nil {
		t.Errorf("error closing form writer: %s", err)
	}
	req, _ = http.NewRequest("POST", "", body)
	req.Header.Add("Content-Type", writer.FormDataContentType())
	q := req.URL.Query()
	q.Add("key", "shouldfail")
	req.URL.RawQuery = q.Encode()
	w = httptest.NewRecorder()
	// should fail
	uploadHandle.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("uploadHandler DELETE returned %v, should be %v", w.Code, http.StatusUnauthorized)
	}

	req, _ = http.NewRequest("POST", "", body)
	req.Header.Add("Content-Type", writer.FormDataContentType())
	tempKey, err := createAPIkey(db)
	if err != nil {
		t.Errorf("error creating API key: %s", err)
	}
	q = req.URL.Query()
	q.Add("key", tempKey)
	req.URL.RawQuery = q.Encode()
	w = httptest.NewRecorder()

	// should pass
	uploadHandle.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("deleteHandler DELETE returned %v, should be %v", w.Code, http.StatusOK)
	}
	// cleanup
	if err := os.RemoveAll(config.RootRepoPath); err != nil {
		t.Errorf("error cleaning up after uploadHandler(): %s", err)
	}
}

func TestDeleteHandler(t *testing.T) {
	pwd, err := os.Getwd()
	if err != nil {
		t.Errorf("Unable to get current working directory: %s", err)
	}
	config := conf{ListenPort: "9666", RootRepoPath: pwd + "/testing", SupportArch: []string{"cats", "dogs"}, DistroNames: []string{"stable"}, Sections: []string{"main", "blah"}, EnableSSL: false}
	// sanity check...
	if config.RootRepoPath != pwd+"/testing" {
		t.Errorf("RootRepoPath is %s, should be %s\n ", config.RootRepoPath, pwd+"/testing")
	}

	// create temp db
	db, err := bolt.Open(tempfile(), 0666, nil)
	if err != nil {
		t.Fatalf("error creating tempdb: %s", err)
	}
	defer db.Close()

	deleteHandle := deleteHandler(config, db)

	// GET
	req, _ := http.NewRequest("GET", "", nil)
	w := httptest.NewRecorder()
	deleteHandle.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("deleteHandler GET returned %v, should be %v", w.Code, http.StatusMethodNotAllowed)
	}

	// DELETE
	// create "all" arch as it's the default
	if err := os.MkdirAll(config.RootRepoPath+"/dists/stable/main/binary-all", 0755); err != nil {
		t.Errorf("error creating directory for POST testing: %s", err)
	}
	tempDeb, err := os.Create(config.RootRepoPath + "/dists/stable/main/binary-all/myapp.deb")
	if err != nil {
		t.Fatalf("create %s: %s", config.RootRepoPath+"/dists/stable/main/binary-all/myapp.deb", err)
	}
	defer tempDeb.Close()
	req, _ = http.NewRequest("DELETE", "", bytes.NewBufferString("{\"filename\":\"myapp.deb\",\"arch\":\"all\", \"distroName\":\"stable\", \"section\":\"main\"}"))
	w = httptest.NewRecorder()
	deleteHandle.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("deleteHandler DELETE returned %v, should be %v", w.Code, http.StatusOK)
	}

	// cleanup
	if err := os.RemoveAll(config.RootRepoPath); err != nil {
		t.Errorf("error cleaning up after uploadHandler(): %s", err)
	}

	// create temp file
	tempFile, err := os.Create(pwd + "/tempFile")
	if err != nil {
		t.Fatalf("create %s: %s", pwd+"/tempFile", err)
	}
	defer tempFile.Close()
	config.RootRepoPath = pwd + "/tempFile"
	// Can't make directory named after file
	deleteHandle = deleteHandler(config, db)
	req, _ = http.NewRequest("DELETE", "", bytes.NewBufferString("{\"filename\":\"myapp.deb\",\"arch\":\"amd64\", \"distroName\":\"stable\", \"section\":\"main\"}"))
	w = httptest.NewRecorder()
	deleteHandle.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("deleteHandler DELETE returned %v, should be %v", w.Code, http.StatusInternalServerError)
	}
	// cleanup
	if err := os.RemoveAll(pwd + "/tempFile"); err != nil {
		t.Errorf("error cleaning up after createDirs(): %s", err)
	}

	// API key tests
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("APIkeys"))
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("error creating db bucket: %s", err)
	}
	config.EnableAPIKeys = true

	// create "all" arch as it's the default
	if err := os.MkdirAll(config.RootRepoPath+"/dists/stable/main/binary-all", 0755); err != nil {
		t.Errorf("error creating directory for POST testing: %s", err)
	}
	tempDeb, err = os.Create(config.RootRepoPath + "/dists/stable/main/binary-all/myapp.deb")
	if err != nil {
		t.Fatalf("create %s: %s", config.RootRepoPath+"/dists/stable/main/binary-all/myapp.deb", err)
	}
	defer tempDeb.Close()
	deleteHandle = deleteHandler(config, db)
	req, _ = http.NewRequest("DELETE", "", bytes.NewBufferString("{\"filename\":\"myapp.deb\",\"arch\":\"all\", \"distroName\":\"stable\", \"section\":\"main\"}"))
	q := req.URL.Query()
	q.Add("key", "shouldfail")
	req.URL.RawQuery = q.Encode()
	w = httptest.NewRecorder()
	deleteHandle.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("deleteHandler DELETE returned %v, should be %v", w.Code, http.StatusUnauthorized)
	}

	// cleanup
	if err := os.RemoveAll(config.RootRepoPath); err != nil {
		t.Errorf("error cleaning up after deleteHandler(): %s", err)
	}

	if err := os.MkdirAll(config.RootRepoPath+"/dists/stable/main/binary-all", 0755); err != nil {
		t.Errorf("error creating directory for POST testing: %s", err)
	}
	tempDeb, err = os.Create(config.RootRepoPath + "/dists/stable/main/binary-all/myapp.deb")
	if err != nil {
		t.Fatalf("create %s: %s", config.RootRepoPath+"/dists/stable/main/binary-all/myapp.deb", err)
	}
	defer tempDeb.Close()
	tempKey, err := createAPIkey(db)
	if err != nil {
		t.Errorf("error creating API key: %s", err)
	}
	req, _ = http.NewRequest("DELETE", "", bytes.NewBufferString("{\"filename\":\"myapp.deb\",\"arch\":\"all\", \"distroName\":\"stable\", \"section\":\"main\"}"))
	q = req.URL.Query()
	q.Add("key", tempKey)
	req.URL.RawQuery = q.Encode()
	w = httptest.NewRecorder()
	deleteHandle.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("deleteHandler DELETE returned %v, should be %v", w.Code, http.StatusOK)
	}

	// cleanup
	if err := os.RemoveAll(config.RootRepoPath); err != nil {
		t.Errorf("error cleaning up after deleteHandler(): %s", err)
	}

}

func TestCreateAPIkey(t *testing.T) {

	// create temp db
	db, err := bolt.Open(tempfile(), 0666, nil)
	if err != nil {
		t.Fatalf("error creating tempdb: %s", err)
	}
	defer db.Close()

	// should fail
	_, err = createAPIkey(db)
	if err == nil {
		t.Errorf("createAPIkey should have failed but didn't")
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("APIkeys"))
		if err != nil {
			//return log.Fatal("unable to create DB bucket: ", err)
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("error creating db bucket: %s", err)
	}

	_, err = createAPIkey(db)
	if err != nil {
		t.Errorf("error creating API key: %s", err)
	}
}

func TestValidateAPIkey(t *testing.T) {

	// create temp db
	db, err := bolt.Open(tempfile(), 0666, nil)
	if err != nil {
		t.Fatalf("error creating tempdb: %s", err)
	}
	defer db.Close()

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("APIkeys"))
		if err != nil {
			//return log.Fatal("unable to create DB bucket: ", err)
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("error creating db bucket: %s", err)
	}

	// should fail
	isValid := validateAPIkey(db, "blah")
	if isValid {
		t.Errorf("validateAPIkey should have returned false but it didn't")
	}
}

func BenchmarkUploadHandler(b *testing.B) {
	pwd, err := os.Getwd()
	if err != nil {
		b.Errorf("Unable to get current working directory: %s", err)
	}
	config := &conf{ListenPort: "9666", RootRepoPath: pwd + "/testing", SupportArch: []string{"cats", "dogs"}, DistroNames: []string{"stable"}, EnableSSL: false}
	// sanity check...
	if config.RootRepoPath != pwd+"/testing" {
		b.Errorf("RootRepoPath is %s, should be %s\n ", config.RootRepoPath, pwd+"/testing")
	}
	// create temp db
	db, err := bolt.Open(tempfile(), 0666, nil)
	if err != nil {
		b.Fatalf("error creating tempdb: %s", err)
	}
	defer db.Close()
	uploadHandle := uploadHandler(*config, db)
	if err := os.MkdirAll(config.RootRepoPath+"/dists/stable/main/binary-all", 0755); err != nil {
		b.Errorf("error creating directory for POST testing: %s", err)
	}
	sampleDeb, err := os.Open("samples/vim-tiny_7.4.052-1ubuntu3_amd64.deb")
	if err != nil {
		b.Errorf("error opening sample deb file: %s", err)
	}
	defer sampleDeb.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// temporary (i hope) hack to solve "http: MultipartReader called twice" error
		b.StopTimer()
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("file", "vim-tiny_7.4.052-1ubuntu3_amd64.deb")
		if err != nil {
			b.Errorf("error FormFile: %s", err)
		}
		if _, err := io.Copy(part, sampleDeb); err != nil {
			b.Errorf("error copying sampleDeb to FormFile: %s", err)
		}
		if err := writer.Close(); err != nil {
			b.Errorf("error closing form writer: %s", err)
		}
		req, _ := http.NewRequest("POST", "/upload?distro=stable", body)
		req.Header.Add("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		b.StartTimer()
		uploadHandle.ServeHTTP(w, req)
	}
	b.StopTimer()
	// cleanup
	_ = os.RemoveAll(config.RootRepoPath)
}

// tempfile returns a temporary file path.
func tempfile() string {
	f, err := ioutil.TempFile("", "bolt-")
	if err != nil {
		panic(err)
	}
	if err := f.Close(); err != nil {
		panic(err)
	}
	if err := os.Remove(f.Name()); err != nil {
		panic(err)
	}
	return f.Name()
}
