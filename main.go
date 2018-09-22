package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	"time"

	"github.com/blakesmith/ar"
	"github.com/boltdb/bolt"
	"github.com/fsnotify/fsnotify"
)

type conf struct {
	ListenPort    string   `json:"listenPort"`
	RootRepoPath  string   `json:"rootRepoPath"`
	SupportArch   []string `json:"supportedArch"`
	Sections      []string `json:"sections"`
	DistroNames   []string `json:"distroNames"`
	EnableSSL     bool     `json:"enableSSL"`
	SSLCert       string   `json:"SSLcert"`
	SSLKey        string   `json:"SSLkey"`
	EnableAPIKeys bool     `json:"enableAPIKeys"`
}

func (c conf) ArchPath(distro, section, arch string) string {
	return filepath.Join(c.RootRepoPath, "dists", distro, section, "binary-"+arch)
}

type deleteObj struct {
	Filename   string
	DistroName string
	Arch       string
	Section    string
}

var (
	mutex        sync.Mutex
	configFile   = flag.String("c", "conf.json", "config file location")
	generateKey  = flag.Bool("g", false, "generate an API key")
	parsedconfig = conf{}
	mywatcher    *fsnotify.Watcher


	//Create a package level time function so we can mock it out
	Now = func() time.Time {
		return time.Now()
	}
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

	var db *bolt.DB
	defer db.Close()

	if parsedconfig.EnableAPIKeys || *generateKey {
		db = openDB()
		// create DB bucket if needed
		err = db.Update(func(tx *bolt.Tx) error {
			_, err := tx.CreateBucketIfNotExists([]byte("APIkeys"))
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			log.Fatal("unable to create database bucket: ", err)
		}
	}
	// generate API key and exit
	if *generateKey {
		fmt.Println("Generating API key...")
		tempKey, err := createAPIkey(db)
		if err != nil {
			log.Fatal("unable to generate API key: ", err)
		}
		fmt.Println("key: ", tempKey)
		os.Exit(0)
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
				if (event.Op&fsnotify.Write == fsnotify.Write) || (event.Op&fsnotify.Remove == fsnotify.Remove) {
					mutex.Lock()
					if filepath.Ext(event.Name) == ".deb" {
						log.Println("Event: ", event)
						distroArch := destructPath(event.Name)
						if err := createPackagesGz(parsedconfig, distroArch[0], distroArch[1], distroArch[2]); err != nil {
							log.Printf("error creating package: %s", err)
						}
						createRelease(parsedconfig, distroArch[0])
					}
					mutex.Unlock()
				}
			case err := <-mywatcher.Errors:
				log.Println("error:", err)
			}
		}
	}()

	http.Handle("/", http.StripPrefix("/", http.FileServer(http.Dir(parsedconfig.RootRepoPath))))
	http.Handle("/upload", uploadHandler(parsedconfig, db))
	http.Handle("/delete", deleteHandler(parsedconfig, db))

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
	section := splitPath[len(splitPath)-3]
	return []string{distro, section, archSplit[1]}
}

func createDirs(config conf) error {
	for _, distro := range config.DistroNames {
		for _, arch := range config.SupportArch {
			for _, section := range config.Sections {
				if _, err := os.Stat(config.ArchPath(distro, section, arch)); err != nil {
					if os.IsNotExist(err) {
						log.Printf("Directory for %s (%s) does not exist, creating", distro, arch)
						if err := os.MkdirAll(config.ArchPath(distro, section, arch), 0755); err != nil {
							return fmt.Errorf("error creating directory for %s (%s): %s", distro, arch, err)
						}
					} else {
						return fmt.Errorf("error inspecting %s (%s): %s", distro, arch, err)
					}
				}
				log.Println("starting watcher for ", config.ArchPath(distro, section, arch))
				err := mywatcher.Add(config.ArchPath(distro, section, arch))
				if err != nil {
					return fmt.Errorf("error creating watcher for %s (%s): %s", distro, arch, err)
				}
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

		if strings.TrimRight(header.Name, "/") == "control.tar.gz" {
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

func createPackagesGz(config conf, distro, section, arch string) error {
	packageFile, err := os.Create(filepath.Join(config.ArchPath(distro, section, arch), "Packages"))
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
			writer.Write(packBuf.Bytes())
			f = nil
		}
	}

	return nil
}

func createRelease(config conf, distro string) error {

	workingDirectory := filepath.Join(config.RootRepoPath, "dists", distro)

	outfile, err := os.Create(filepath.Join(workingDirectory, "Release"))
	if err != nil {
		return fmt.Errorf("failed to create Release: %s", err)
	}
	defer outfile.Close()
	
	current_time := Now().UTC()
    fmt.Fprintf(outfile, "Suite: %s\n", distro)
    fmt.Fprintf(outfile, "Codename: %s\n", distro)
    fmt.Fprintf(outfile, "Components: %s\n", strings.Join(config.Sections, " "))
    fmt.Fprintf(outfile, "Architectures: %s\n", strings.Join(config.SupportArch, " "))
    fmt.Fprintf(outfile, "Date: %s\n", current_time.Format("Mon, 02 Jan 2006 15:04:05 UTC"))

	var md5Sums strings.Builder
	var sha1Sums strings.Builder
	var sha256Sums strings.Builder

	err = filepath.Walk(workingDirectory, func(path string, file os.FileInfo, err error) error {
	    if err != nil {
	        return err
	    }

    	if strings.HasSuffix(path, "Packages.gz") || strings.HasSuffix(path, "Packages") {

    		var relPath string
    		relPath, err := filepath.Rel(workingDirectory, path)
    		spath := filepath.ToSlash(relPath)
    		f, err := os.Open(path)
    		var (
    			md5hash    = md5.New()
    			sha1hash   = sha1.New()
    			sha256hash = sha256.New()
    		)
    		if _, err = io.Copy(io.MultiWriter(md5hash, sha1hash, sha256hash), f); err != nil {
    			log.Println("error with the md5 hash: ", err)
    		}
    		fmt.Fprintf(&md5Sums, " %s %d %s\n",
    			hex.EncodeToString(md5hash.Sum(nil)),
    			file.Size(), spath)

    		if _, err = io.Copy(sha1hash, f); err != nil {
    			log.Println("error with the sha1 hash: ", err)
    		}
    		fmt.Fprintf(&sha1Sums, " %s %d %s\n",
    			hex.EncodeToString(sha1hash.Sum(nil)),
    			file.Size(), spath)

    		if _, err = io.Copy(sha256hash, f); err != nil {
    			log.Println("error with the sha256 hash: ", err)
    		}
    		fmt.Fprintf(&sha256Sums, " %s %d %s\n",
    			hex.EncodeToString(sha256hash.Sum(nil)),
    			file.Size(), spath)

    		f = nil
    	}
	    return nil
	})

	if err != nil {
		return fmt.Errorf("scanning: %s: %s", distro, err)
	}

	outfile.WriteString("MD5Sum:\n")
	outfile.WriteString(md5Sums.String())
	outfile.WriteString("SHA1:\n")
	outfile.WriteString(sha1Sums.String())
	outfile.WriteString("SHA256:\n")
	outfile.WriteString(sha256Sums.String())

	return nil
}
func uploadHandler(config conf, db *bolt.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not supported", http.StatusMethodNotAllowed)
			return
		}
		if config.EnableAPIKeys {
			apiKey := r.URL.Query().Get("key")
			if apiKey == "" {
				http.Error(w, "api key not present", http.StatusUnauthorized)
				return
			}
			if !validateAPIkey(db, apiKey) {
				http.Error(w, "api key not valid", http.StatusUnauthorized)
				return
			}
		}
		archType := r.URL.Query().Get("arch")
		if archType == "" {
			archType = "all"
		}
		distroName := r.URL.Query().Get("distro")
		if distroName == "" {
			distroName = "stable"
		}
		section := r.URL.Query().Get("section")
		if section == "" {
			section = "main"
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
			dst, err := os.Create(filepath.Join(config.ArchPath(distroName, section, archType), part.FileName()))
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

func deleteHandler(config conf, db *bolt.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			http.Error(w, "method not supported", http.StatusMethodNotAllowed)
			return
		}
		if config.EnableAPIKeys {
			apiKey := r.URL.Query().Get("key")
			if apiKey == "" {
				http.Error(w, "api key not present", http.StatusUnauthorized)
				return
			}
			if !validateAPIkey(db, apiKey) {
				http.Error(w, "api key not valid", http.StatusUnauthorized)
				return
			}
		}
		var toDelete deleteObj
		if err := json.NewDecoder(r.Body).Decode(&toDelete); err != nil {
			httpErrorf(w, "failed to decode json: %s", err)
			return
		}
		debPath := filepath.Join(config.ArchPath(toDelete.DistroName, toDelete.Section, toDelete.Arch), toDelete.Filename)
		if err := os.Remove(debPath); err != nil {
			httpErrorf(w, "failed to delete: %s", err)
			return
		}
	})
}

func openDB() *bolt.DB {
	// open/create database for API keys
	db, err := bolt.Open("debsimple.db", 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Fatal("unable to open database: ", err)
	}

	return db
}

func createAPIkey(db *bolt.DB) (string, error) {
	randomBytes := make([]byte, 32)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}
	apiKey := base64.URLEncoding.EncodeToString(randomBytes)

	err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("APIkeys"))
		if b == nil {
			return errors.New("database bucket does not exist!")
		}

		err = b.Put([]byte(apiKey), []byte(apiKey))
		return err
	})
	if err != nil {
		return "", err
	} else {
		return apiKey, nil
	}
}

func validateAPIkey(db *bolt.DB, key string) bool {
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("APIkeys"))
		v := b.Get([]byte(key))
		if len(v) == 0 {
			return errors.New("key not found")
		}
		return nil
	})
	if err == nil {
		return true
	} else {
		return false
	}
}

func httpErrorf(w http.ResponseWriter, format string, a ...interface{}) {
	err := fmt.Errorf(format, a...)
	log.Println(err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
