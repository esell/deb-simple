package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/blakesmith/ar"
	"github.com/fsnotify/fsnotify"
)

type conf struct {
	ListenPort   string   `json:"listenPort"`
	RootRepoPath string   `json:"rootRepoPath"`
	SupportArch  []string `json:"supportedArch"`
	DistroNames  []string `json:"distroNames"`
	EnableSSL    bool     `json:"enableSSL"`
	SSLCert      string   `json:"SSLcert"`
	SSLKey       string   `json:"SSLkey"`
}

func (c conf) ArchPath(distro, arch string) string {
	return filepath.Join(c.RootRepoPath, "dists", distro, "main/binary-"+arch)
}

type deleteObj struct {
	Filename   string
	DistroName string
	Arch       string
}

var (
	mutex        sync.Mutex
	configFile   = flag.String("c", "conf.json", "config file location")
	parsedconfig = conf{}
	mywatcher    *fsnotify.Watcher
)

func main() {
	flag.Parse()
	file, err := ioutil.ReadFile(*configFile)
	if err != nil {
		log.Fatal("unable to read config file, exiting...")
	}
	if err := json.Unmarshal(file, &parsedconfig); err != nil {
		log.Fatal("unable to marshal config file, exiting...")
	}

	// fire up filesystem watcher
	mywatcher, err = fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("error creating fswatcher: ", err)
	}

	if err := createDirs(parsedconfig); err != nil {
		log.Println(err)
		log.Fatalf("error creating directory structure, exiting")
	}

	go func() {
		for {
			select {
			case event := <-mywatcher.Events:
				if (event.Op&fsnotify.Write == fsnotify.Write) || (event.Op&fsnotify.Create == fsnotify.Create) || (event.Op&fsnotify.Remove == fsnotify.Remove) {
					log.Println("modified file:", event.Name)
					mutex.Lock()

					log.Println("got lock, updating package list...")
					distroArch := destructPath(event.Name)
					if filepath.Base(event.Name) != "Packages.gz" {
						if err := createPackagesGz(parsedconfig, distroArch[0], distroArch[1]); err != nil {
							log.Println("error creating package: %s", err)
						}
					}
					mutex.Unlock()
				}
			case err := <-mywatcher.Errors:
				log.Println("error:", err)
			}
		}
	}()

	http.Handle("/", http.StripPrefix("/", http.FileServer(http.Dir(parsedconfig.RootRepoPath))))
	http.Handle("/upload", uploadHandler(parsedconfig))
	http.Handle("/delete", deleteHandler(parsedconfig))

	if parsedconfig.EnableSSL {
		log.Println("running with SSL enabled")
		log.Fatal(http.ListenAndServeTLS(":"+parsedconfig.ListenPort, parsedconfig.SSLCert, parsedconfig.SSLKey, nil))
	} else {
		log.Println("running without SSL enabled")
		log.Fatal(http.ListenAndServe(":"+parsedconfig.ListenPort, nil))
	}
}

func destructPath(filePath string) []string {
	splitPath := strings.Split(filePath, "/")
	archFull := splitPath[len(splitPath)-2]
	archSplit := strings.Split(archFull, "-")
	distro := splitPath[len(splitPath)-4]
	return []string{distro, archSplit[1]}
}

func createDirs(config conf) error {
	for _, distro := range config.DistroNames {
		for _, arch := range config.SupportArch {
			if _, err := os.Stat(config.ArchPath(distro, arch)); err != nil {
				if os.IsNotExist(err) {
					log.Printf("Directory for %s (%s) does not exist, creating", distro, arch)
					if err := os.MkdirAll(config.ArchPath(distro, arch), 0755); err != nil {
						return fmt.Errorf("error creating directory for %s (%s): %s", distro, arch, err)
					}
					//log.Println("starting watcher for ", config.ArchPath(distro, arch))
					//err := mywatcher.Add(config.ArchPath(distro, arch))
					//if err != nil {
					//	return fmt.Errorf("error creating watcher for %s (%s): %s", distro, arch, err)
					//}
				} else {
					return fmt.Errorf("error inspecting %s (%s): %s", distro, arch, err)
				}
			}
			log.Println("starting watcher for ", config.ArchPath(distro, arch))
			err := mywatcher.Add(config.ArchPath(distro, arch))
			if err != nil {
				return fmt.Errorf("error creating watcher for %s (%s): %s", distro, arch, err)
			}
		}
	}
	return nil
}

func inspectPackage(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", fmt.Errorf("error opening package file %s: %s", filename, err)
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

		if header.Name == "control.tar.gz" {
			io.Copy(&controlBuf, arReader)
			return inspectPackageControl(controlBuf)
		}

	}
	return "", nil
}

func inspectPackageControl(filename bytes.Buffer) (string, error) {
	gzf, err := gzip.NewReader(bytes.NewReader(filename.Bytes()))
	if err != nil {
		return "", fmt.Errorf("error creating gzip reader: %s", err)
	}

	tarReader := tar.NewReader(gzf)
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
			if name == "./control" {
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

func createPackagesGz(config conf, distro, arch string) error {
	outfile, err := os.Create(filepath.Join(config.ArchPath(distro, arch), "Packages.gz"))
	if err != nil {
		return fmt.Errorf("failed to create packages.gz: %s", err)
	}
	defer outfile.Close()
	gzOut := gzip.NewWriter(outfile)
	defer gzOut.Close()
	// loop through each directory
	// run inspectPackage
	dirList, err := ioutil.ReadDir(config.ArchPath(distro, arch))
	if err != nil {
		return fmt.Errorf("scanning: %s: %s", config.ArchPath(distro, arch), err)
	}
	for i, debFile := range dirList {
		if strings.HasSuffix(debFile.Name(), "deb") {
			var packBuf bytes.Buffer
			debPath := filepath.Join(config.ArchPath(distro, arch), debFile.Name())
			tempCtlData, err := inspectPackage(debPath)
			if err != nil {
				return err
			}
			packBuf.WriteString(tempCtlData)
			dir := filepath.Join("dists", distro, "main/binary-"+arch, debFile.Name())
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
				log.Println("error with the md5 hash: ", err)
			}
			fmt.Fprintf(&packBuf, "MD5sum: %s\n",
				hex.EncodeToString(md5hash.Sum(nil)))
			if _, err = io.Copy(sha1hash, f); err != nil {
				log.Println("error with the sha1 hash: ", err)
			}
			fmt.Fprintf(&packBuf, "SHA1: %s\n",
				hex.EncodeToString(sha1hash.Sum(nil)))
			if _, err = io.Copy(sha256hash, f); err != nil {
				log.Println("error with the sha256 hash: ", err)
			}
			fmt.Fprintf(&packBuf, "SHA256: %s\n",
				hex.EncodeToString(sha256hash.Sum(nil)))
			if i != (len(dirList) - 1) {
				packBuf.WriteString("\n\n")
			}
			gzOut.Write(packBuf.Bytes())
			f = nil
		}
	}

	gzOut.Flush()
	return nil
}

func uploadHandler(config conf) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not supported", http.StatusMethodNotAllowed)
			return
		}
		archType := r.URL.Query().Get("arch")
		if archType == "" {
			archType = "all"
		}
		distroName := r.URL.Query().Get("distro")
		if distroName == "" {
			distroName = "stable"
		}
		reader, err := r.MultipartReader()
		if err != nil {
			httpErrorf(w, "error creating multipart reader: %s", err)
			return
		}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if part.FileName() == "" {
				continue
			}
			dst, err := os.Create(filepath.Join(config.ArchPath(distroName, archType), part.FileName()))
			if err != nil {
				httpErrorf(w, "error creating deb file: %s", err)
				return
			}
			defer dst.Close()
			if _, err := io.Copy(dst, part); err != nil {
				httpErrorf(w, "error writing deb file: %s", err)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	})
}

func deleteHandler(config conf) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			http.Error(w, "method not supported", http.StatusMethodNotAllowed)
			return
		}
		var toDelete deleteObj
		if err := json.NewDecoder(r.Body).Decode(&toDelete); err != nil {
			httpErrorf(w, "failed to decode json: %s", err)
			return
		}
		debPath := filepath.Join(config.ArchPath(toDelete.DistroName, toDelete.Arch), toDelete.Filename)
		if err := os.Remove(debPath); err != nil {
			httpErrorf(w, "failed to delete: %s", err)
			return
		}
	})
}

func httpErrorf(w http.ResponseWriter, format string, a ...interface{}) {
	err := fmt.Errorf(format, a...)
	log.Println(err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
