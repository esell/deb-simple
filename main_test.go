package main

import (
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/boltdb/bolt"
	"github.com/fsnotify/fsnotify"
)

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
