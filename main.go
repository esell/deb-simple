package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/fsnotify/fsnotify"
)

type conf struct {
	ListenPort              string   `json:"listenPort"`
	RootRepoPath            string   `json:"rootRepoPath"`
	SupportArch             []string `json:"supportedArch"`
	Sections                []string `json:"sections"`
	DistroNames             []string `json:"distroNames"`
	EnableSSL               bool     `json:"enableSSL"`
	SSLCert                 string   `json:"SSLcert"`
	SSLKey                  string   `json:"SSLkey"`
	EnableAPIKeys           bool     `json:"enableAPIKeys"`
	EnableSigning           bool     `json:"enableSigning"`
	PrivateKey              string   `json:"privateKey"`
	EnableDirectoryWatching bool     `json:"enableDirectoryWatching"`
}

func (c conf) ArchPath(distro, section, arch string) string {
	return filepath.Join(c.RootRepoPath, "dists", distro, section, "binary-"+arch)
}

var (
	mutex              sync.Mutex
	configFile         = flag.String("c", "conf.json", "config file location")
	generateKey        = flag.Bool("g", false, "generate an API key")
	generateSigningKey = flag.Bool("k", false, "Generate a signing key pair")
	keyName            = flag.String("kn", "", "Name for the siging key")
	keyEmail           = flag.String("ke", "", "Email address")
	verbose            = flag.Bool("v", false, "Print verbose logs")
	parsedconfig       = conf{}
	mywatcher          *fsnotify.Watcher

	// Now is a package level time function so we can mock it out
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

	if *generateSigningKey {

		workingDirectory, err := os.Getwd()
		if err != nil {
			log.Fatalf("Unable to get current working directory: %s", err)
		}

		fmt.Println("Generating new signing key pair..")
		fmt.Printf("Name: %s\n", *keyName)
		fmt.Printf("Email: %s\n", *keyEmail)
		createKeyHandler(workingDirectory, *keyName, *keyEmail)
		fmt.Println("Done.")
		os.Exit(0)
	}

	if parsedconfig.EnableDirectoryWatching {

		// fire up filesystem watcher
		mywatcher, err = fsnotify.NewWatcher()
		if err != nil {
			log.Fatal("error creating fswatcher: ", err)
		}

		go func() {
			for {
				select {
				case event := <-mywatcher.Events:
					if (event.Op&fsnotify.Write == fsnotify.Write) || (event.Op&fsnotify.Remove == fsnotify.Remove) {
						mutex.Lock()
						if filepath.Ext(event.Name) == ".deb" {
							if *verbose {
								log.Println("Event: ", event)
							}
							rebuildRepoMetadata(event.Name)
						}
						mutex.Unlock()
					}
				case err := <-mywatcher.Errors:
					log.Println("error:", err)
				}
			}
		}()
	}

	if err := createDirs(parsedconfig); err != nil {
		log.Println(err)
		log.Fatalf("error creating directory structure, exiting")
	}

	http.Handle("/", http.StripPrefix("/", http.FileServer(http.Dir(parsedconfig.RootRepoPath))))
	http.Handle("/upload", uploadHandler(parsedconfig, db))
	http.Handle("/delete", deleteHandler(parsedconfig, db))

	if parsedconfig.EnableSigning {
		log.Println("Release signing is enabled")
	}

	if parsedconfig.EnableSSL {
		log.Println("running with SSL enabled")
		log.Fatal(http.ListenAndServeTLS(":"+parsedconfig.ListenPort, parsedconfig.SSLCert, parsedconfig.SSLKey, nil))
	} else {
		log.Println("running without SSL enabled")
		log.Fatal(http.ListenAndServe(":"+parsedconfig.ListenPort, nil))
	}
}

func rebuildRepoMetadata(filePath string) {
	distroArch := destructPath(filePath)
	if err := createPackagesGz(parsedconfig, distroArch[0], distroArch[1], distroArch[2]); err != nil {
		log.Printf("error creating Packages file: %s", err)
	}
	if parsedconfig.EnableSigning {
		if err := createRelease(parsedconfig, distroArch[0]); err != nil {
			log.Printf("Error creating Release file: %s", err)
		}
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
				if parsedconfig.EnableDirectoryWatching {
					log.Println("starting watcher for ", config.ArchPath(distro, section, arch))
					err := mywatcher.Add(config.ArchPath(distro, section, arch))
					if err != nil {
						return fmt.Errorf("error creating watcher for %s (%s): %s", distro, arch, err)
					}
				}
			}
		}
	}
	return nil
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
			return errors.New("database bucket \"APIkeys\" does not exist")
		}

		err = b.Put([]byte(apiKey), []byte(apiKey))
		return err
	})
	if err != nil {
		return "", err
	}
	return apiKey, nil
}
