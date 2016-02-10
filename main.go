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
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/blakesmith/ar"
)

type semaphore chan int

type Conf struct {
	ListenPort   string   `json:"listenPort"`
	RootRepoPath string   `json:"rootRepoPath"`
	SupportArch  []string `json:"supportedArch"`
	EnableSSL    bool     `json:"enableSSL"`
	SSLCert      string   `json:"SSLcert"`
	SSLKey       string   `json:"SSLkey"`
}

type DeleteObj struct {
	Filename string
	Arch     string
}

var sem = make(semaphore, 1)
var configFile = flag.String("c", "conf.json", "config file location")
var parsedConfig = &Conf{}

func main() {
	flag.Parse()
	file, err := ioutil.ReadFile(*configFile)
	if err != nil {
		log.Fatal("unable to read config file, exiting...")
	}
	//config = &Conf{}
	if err := json.Unmarshal(file, &parsedConfig); err != nil {
		log.Fatal("unable to marshal config file, exiting...")
	}

	if !createDirs(*parsedConfig) {
		log.Fatal("error creating directory structure, exiting")
	}
	http.Handle("/", http.StripPrefix("/", http.FileServer(http.Dir(parsedConfig.RootRepoPath))))
	http.Handle("/upload", uploadHandler(*parsedConfig))
	http.Handle("/delete", deleteHandler(*parsedConfig))
	if parsedConfig.EnableSSL {
		log.Println("running with SSL enabled")
		log.Fatal(http.ListenAndServeTLS(":"+parsedConfig.ListenPort, parsedConfig.SSLCert, parsedConfig.SSLKey, nil))
	} else {
		log.Println("running without SSL enabled")
		log.Fatal(http.ListenAndServe(":"+parsedConfig.ListenPort, nil))
	}
}

func createDirs(config Conf) bool {
	for _, archDir := range config.SupportArch {
		if _, err := os.Stat(config.RootRepoPath + "/dists/stable/main/binary-" + archDir); err != nil {
			if os.IsNotExist(err) {
				log.Printf("Directory for %s does not exist, creating", archDir)
				dirErr := os.MkdirAll(config.RootRepoPath+"/dists/stable/main/binary-"+archDir, 0755)
				if dirErr != nil {
					log.Printf("error creating directory for %s: %s\n", archDir, dirErr)
					return false
				}
			} else {
				log.Printf("error inspecting %s: %s\n", archDir, err)
				return false
			}
		}
	}
	return true
}

func inspectPackage(filename string) string {
	f, err := os.Open(filename)
	if err != nil {
		log.Printf("error opening package file %s: %s\n", filename, err)
		return ""
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
			log.Println("error in inspectPackage loop: ", err)
			return ""
		}

		if header.Name == "control.tar.gz" {
			io.Copy(&controlBuf, arReader)
			return inspectPackageControl(controlBuf)

		}

	}
	return ""
}

func inspectPackageControl(filename bytes.Buffer) string {
	gzf, err := gzip.NewReader(bytes.NewReader(filename.Bytes()))
	if err != nil {
		log.Println("error creating gzip reader: ", err)
		return ""
	}

	tarReader := tar.NewReader(gzf)
	var controlBuf bytes.Buffer
	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			log.Println("error in inspectPackage loop: ", err)
			return ""
		}

		name := header.Name

		switch header.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
			if name == "./control" {
				io.Copy(&controlBuf, tarReader)
				return controlBuf.String()
			}
		default:
			log.Printf("%s : %c %s %s\n",
				"Unable to figure out type",
				header.Typeflag,
				"in file",
				name,
			)
		}
	}
	return ""
}

func createPackagesGz(config *Conf, arch string) bool {
	outfile, err := os.Create(config.RootRepoPath + "/dists/stable/main/binary-" + arch + "/Packages.gz")
	if err != nil {
		log.Println("error creating packages.gz file")
		return false
	}
	defer outfile.Close()
	gzOut := gzip.NewWriter(outfile)
	defer gzOut.Close()
	// loop through each directory
	// run inspectPackage
	dirList, err := ioutil.ReadDir(config.RootRepoPath + "/dists/stable/main/binary-" + arch)
	if err != nil {
		log.Printf("error scanning %s: %s\n", config.RootRepoPath+"/dists/stable/main/binary-"+arch, err)
		return false
	}
	for _, debFile := range dirList {
		if strings.HasSuffix(debFile.Name(), "deb") {
			var packBuf bytes.Buffer
			tempCtlData := inspectPackage(config.RootRepoPath + "/dists/stable/main/binary-" + arch + "/" + debFile.Name())
			packBuf.WriteString(tempCtlData)
			packBuf.WriteString("Filename: " + "dists/stable/main/binary-" + arch + "/" + debFile.Name() + "\n")
			packBuf.WriteString("Size: " + strconv.FormatInt(debFile.Size(), 10) + "\n")
			f, err := os.Open(config.RootRepoPath + "/dists/stable/main/binary-" + arch + "/" + debFile.Name())
			if err != nil {
				log.Println("error opening deb file: ", err)
			}
			defer f.Close()
			md5hash := md5.New()
			sha1hash := sha1.New()
			sha256hash := sha256.New()
			_, err = io.Copy(io.MultiWriter(md5hash, sha1hash, sha256hash), f)
			if err != nil {
				log.Println("error with the md5 hash: ", err)
			}
			md5sum := hex.EncodeToString(md5hash.Sum(nil))
			packBuf.WriteString("MD5sum: " + md5sum + "\n")
			_, err = io.Copy(sha1hash, f)
			if err != nil {
				log.Println("error with the sha1 hash: ", err)
			}
			sha1sum := hex.EncodeToString(sha1hash.Sum(nil))
			packBuf.WriteString("SHA1: " + sha1sum + "\n")
			_, err = io.Copy(sha256hash, f)
			if err != nil {
				log.Println("error with the sha256 hash: ", err)
			}
			sha256sum := hex.EncodeToString(sha256hash.Sum(nil))
			packBuf.WriteString("SHA256: " + sha256sum + "\n")
			packBuf.WriteString("\n\n")
			gzOut.Write(packBuf.Bytes())
			f = nil
		}
	}

	gzOut.Flush()
	return true
}

func uploadHandler(config Conf) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			queryVals := r.URL.Query()
			archType := "all"
			if queryVals.Get("arch") != "" {
				archType = queryVals.Get("arch")
			}
			reader, err := r.MultipartReader()
			if err != nil {
				log.Println("error creating multipart reader: ", err)
				return
			}
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					log.Println("breaking")
					break
				}
				log.Println("form part: ", part.FormName())
				if part.FileName() == "" {
					continue
				}
				log.Println("found file: ", part.FileName())
				log.Println("creating files: ", part.FileName())
				dst, err := os.Create(config.RootRepoPath + "/dists/stable/main/binary-" + archType + "/" + part.FileName())
				defer dst.Close()
				if err != nil {
					log.Println("error creating deb file: ", err)
					//http.Error(w, err.Error(), http.StatusInternalServerError)
					//return
				}
				if _, err := io.Copy(dst, part); err != nil {
					log.Println("error writing deb file: ", err)
					///http.Error(w, err.Error(), http.StatusInternalServerError)
					//return
				}
			}

			log.Println("grabbing lock...")
			sem.Lock()
			log.Println("got lock, updating package list...")
			createPkgRes := createPackagesGz(&config, archType)
			if !createPkgRes {
				log.Println("unable to create Packages.gz")
			}
			sem.Unlock()
			log.Println("lock returned")
			w.WriteHeader(http.StatusOK)
		} else {
			log.Println("not a POST")
			return
		}
	})
}

func deleteHandler(config Conf) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			decoder := json.NewDecoder(r.Body)
			var toDelete DeleteObj
			err := decoder.Decode(&toDelete)
			if err != nil {
				log.Println("error decoding DELETE json: ", err)
			}
			err = os.Remove(config.RootRepoPath + "/dists/stable/main/binary-" + toDelete.Arch + "/" + toDelete.Filename)
			log.Println("grabbing lock...")
			sem.Lock()
			log.Println("got lock, updating package list...")
			if !createPackagesGz(&config, toDelete.Arch) {
				log.Println("unable to create Packages.gz")
			}
			sem.Unlock()
			log.Println("lock returned")
		} else {
			log.Println("not a DELETE")
		}
	})
}

func (s semaphore) Lock() {
	s <- 1
}

func (s semaphore) Unlock() {
	<-s
}
