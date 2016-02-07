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
		log.Panic("unable to read config file, exiting...")
	}
	//config = &Conf{}
	if err := json.Unmarshal(file, &parsedConfig); err != nil {
		log.Panic("unable to marshal config file, exiting...")
	}

	if !createDirs(*parsedConfig) {
		log.Println("error creating directory structure, exiting")
		os.Exit(1)
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
				log.Printf("control file: %s\n", controlBuf.String())
				return controlBuf.String()
			}
		default:
			log.Printf("%s : %c %s %s\n",
				"Yikes! Unable to figure out type",
				header.Typeflag,
				"in file",
				name,
			)
		}
	}
	return ""
}

func createPackagesGz(config *Conf, arch string) bool {
	var packBuf bytes.Buffer
	// loop through each directory
	// run inspectPackage
	dirList, err := ioutil.ReadDir(config.RootRepoPath + "/dists/stable/main/binary-" + arch)
	if err != nil {
		log.Printf("error scanning %s: %s\n", config.RootRepoPath+"/dists/stable/main/binary-"+arch, err)
		return false
	}
	for _, debFile := range dirList {
		if strings.HasSuffix(debFile.Name(), "deb") {
			tempCtlData := inspectPackage(config.RootRepoPath + "/dists/stable/main/binary-" + arch + "/" + debFile.Name())
			packBuf.WriteString(tempCtlData)
			packBuf.WriteString("Filename: " + "dists/stable/main/binary-" + arch + "/" + debFile.Name() + "\n")
			packBuf.WriteString("Size: " + strconv.FormatInt(debFile.Size(), 10) + "\n")
			f, _ := ioutil.ReadFile(config.RootRepoPath + "/dists/stable/main/binary-" + arch + "/" + debFile.Name())
			md5hash := md5.New()
			io.WriteString(md5hash, string(f[:]))
			md5sum := hex.EncodeToString(md5hash.Sum(nil))
			packBuf.WriteString("MD5sum: " + md5sum + "\n")
			sha1hash := sha1.New()
			io.WriteString(sha1hash, string(f[:]))
			sha1sum := hex.EncodeToString(sha1hash.Sum(nil))
			packBuf.WriteString("SHA1: " + sha1sum + "\n")
			sha256hash := sha256.New()
			io.WriteString(sha256hash, string(f[:]))
			sha256sum := hex.EncodeToString(sha256hash.Sum(nil))
			packBuf.WriteString("SHA256: " + sha256sum + "\n")
			packBuf.WriteString("\n\n")
		}
	}

	outfile, err := os.Create(config.RootRepoPath + "/dists/stable/main/binary-" + arch + "/Packages.gz")
	if err != nil {
		log.Println("error creating packages.gz file")
		return false
	}
	gzOut := gzip.NewWriter(outfile)
	gzOut.Write(packBuf.Bytes())
	gzOut.Flush()
	defer outfile.Close()
	defer gzOut.Close()
	return true
}

func uploadHandler(config Conf) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			archType := r.FormValue("arch")
			if archType == "" {
				archType = "all"
			}
			log.Println("archType is: ", archType)
			file, header, err := r.FormFile("file")
			if err != nil {
				log.Println("error parsing file: ", err)
			}
			log.Println("filename is: ", header.Filename)
			out, err := os.Create(config.RootRepoPath + "/dists/stable/main/binary-" + archType + "/" + header.Filename)
			if err != nil {
				log.Println("error creating file: ", err)
			}
			_, err = io.Copy(out, file)
			if err != nil {
				log.Println("error saving file: ", err)
			}
			defer file.Close()
			defer out.Close()
			log.Println("grabbing lock...")
			sem.Lock()
			log.Println("got lock, updating package list...")
			if !createPackagesGz(&config, archType) {
				log.Println("unable to create Packages.gz")
			}
			sem.Unlock()
			log.Println("lock returned")
		} else {
			log.Println("not a POST")
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
