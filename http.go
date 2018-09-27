package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/boltdb/bolt"
)

type deleteObj struct {
	Filename   string
	DistroName string
	Arch       string
	Section    string
}

func uploadHandler(config conf, db *bolt.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
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

			if *verbose {
				log.Printf("Deb package %s has been uploaded to %s %s %s", part.FileName(), distroName, section, archType)
			}

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
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
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

		if *verbose {
			log.Printf("Deb package %s has been deleted", toDelete.Filename)
		}
	})
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
	return err == nil
}

func httpErrorf(w http.ResponseWriter, format string, a ...interface{}) {
	err := fmt.Errorf(format, a...)
	log.Println(err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
