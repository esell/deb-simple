package main

type Conf struct {
	ListenPort      string   `json:"listenPort"`
	RootRepoPath    string   `json:"rootRepoPath"`
	SupportArch     []string `json:"supportedArch"`
	ScanpackagePath string   `json:"scanpackagePath"`
	GzipPath        string   `json:"gzipPath"`
}
