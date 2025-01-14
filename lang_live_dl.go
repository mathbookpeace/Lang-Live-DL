package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/gen2brain/beeep"
)

type memberData struct {
	Id           int     `json:"id"`
	Name         string  `json:"name"`
	EnableNotify bool    `json:"enable_notify"`
	IconPath     string  `json:"icon"`
	Folder       *string `json:"folder"`
	Prefix       *string `json:"prefix"`
}

type defaultConfig struct {
	DefaultFolder *string `json:"default_folder"`
	CheckOnly     bool    `json:"check_only"`
}

type configs struct {
	DefaultConfigs defaultConfig `json:"default_configs"`
	Members        []memberData  `json:"members"`
}

type streamSource struct {
	name         string
	url          string
	filename     string
	fileFolder   string
	checkOnly    bool
	enableNotify bool
}

const (
	defaultFolderPath = "data"
	tempFolderPath    = "temp"

	defaultConfigJson = "default_config.json"
	configJson        = "config.json"
)

func main() {
	fmt.Println("love kuri")

	readConfig()
	createFolders()

	configs := readConfig()
	sources := buildStreamSources(configs)
	downloadTable := initDownloadTable(sources)

	startDownloadThread(sources, downloadTable)
}

func readConfig() configs {
	data, err := os.ReadFile(decideConfigFile())
	if err != nil {
		panic(err)
	}
	configs := configs{}
	if err := json.Unmarshal(data, &configs); err != nil {
		panic(err)
	}
	return configs
}

func decideConfigFile() string {
	if _, err := os.Stat(configJson); !os.IsNotExist(err) {
		return configJson
	}
	return defaultConfigJson
}

func createFolders() {
	if err := os.MkdirAll(defaultFolderPath, os.ModePerm); err != nil {
		panic(fmt.Sprintf("create folder error, path = %v, err = %v\n", defaultFolderPath, err))
	}
	if err := os.MkdirAll(tempFolderPath, os.ModePerm); err != nil {
		panic(fmt.Sprintf("create folder error, path = %v, err = %v\n", defaultFolderPath, err))
	}
}

func initDownloadTable(sources []streamSource) map[string]bool {
	downloadTable := map[string]bool{}
	for _, src := range sources {
		downloadTable[src.url] = false
	}
	return downloadTable
}

func buildStreamSources(configs configs) []streamSource {
	var sources []streamSource
	domains := []string{"video-ws-aws", "video-ws-hls-aws", "video-tx-int", "audio-tx-lh2"}
	for _, member := range configs.Members {
		idx := 0
		for _, domain := range domains {
			for _, filenamePostfix := range []string{"Y", "A"} {
				sources = append(sources, streamSource{
					name:         member.Name,
					url:          fmt.Sprintf("https://%v.lv-play.com/live/%v%v.flv", domain, member.Id, filenamePostfix),
					filename:     fmt.Sprintf("%vi%v.", decideFilePrefix(&member), idx),
					fileFolder:   decideFileFolder(&member, &configs.DefaultConfigs),
					checkOnly:    configs.DefaultConfigs.CheckOnly,
					enableNotify: member.EnableNotify,
				})
				idx += 1
			}
		}
	}
	return sources
}

func startDownloadThread(sources []streamSource, downloadTable map[string]bool) {
	var downloadTableMtx sync.Mutex
	threadCount := 0
	threadLimit := len(sources)
	for {
		for _, src := range sources {
			func() {
				downloadTableMtx.Lock()
				defer downloadTableMtx.Unlock()
				if !downloadTable[src.url] {
					downloadTable[src.url] = true
					threadCount += 1
					if threadCount > threadLimit {
						fmt.Printf("current thread cnt = %v\n", threadCount)
					}
					go func(src streamSource) {
						downloadForMember(&src)
						downloadTableMtx.Lock()
						defer downloadTableMtx.Unlock()
						downloadTable[src.url] = false
						threadCount -= 1
					}(src)
				}
			}()
		}
		time.Sleep(2 * time.Second)
	}
}

func downloadForMember(src *streamSource) {
	filename := fmt.Sprintf("%v%v", src.filename, time.Now().Format("2006.01.02 15.04.05"))
	tempOutfilePath := filepath.Join(tempFolderPath, fmt.Sprintf("%v.flv", filename))
	if !downloadVideo(src, tempOutfilePath) {
		return
	}
	outfilePath := filepath.Join(src.fileFolder, fmt.Sprintf("%v.mp4", filename))
	if !src.checkOnly {
		if err := flvToMp4(tempOutfilePath, outfilePath); err != nil {
			fmt.Printf("flv to mp4 failed, err = %v\n", err)
			return
		}
	} else {
		if err := os.Remove(tempOutfilePath); err != nil {
			fmt.Printf("remove file failed, err = %v", err)
			return
		}
	}
	fmt.Printf("download completed, name = %v\n", src.name)
}

func downloadVideo(src *streamSource, outfilePath string) bool {
	resp, err := http.Get(src.url)
	if err != nil {
		fmt.Printf("http get failed, name = %v, err = %v\n", src.name, err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return false
	} else if resp.StatusCode != 200 {
		fmt.Printf("status = %v\n", resp.Status)
		return false
	}
	fmt.Println(src)
	if !writeRespToFile(src, outfilePath, resp) {
		return false
	}
	return true
}

func writeRespToFile(src *streamSource, outfilePath string, resp *http.Response) bool {
	outfile, err := os.Create(outfilePath)
	if err != nil {
		fmt.Printf("create file error, path = %v, err = %v\n", outfilePath, err)
		return false
	}
	defer outfile.Close()

	if src.enableNotify {
		if err := beeep.Alert("stream start", src.name, ""); err != nil {
			fmt.Printf("Alert error = %v", err)
		}
	}
	fmt.Printf("stream start %v\n", src.name)

	n, err := io.Copy(outfile, resp.Body)
	if err != nil {
		fmt.Printf("download failed, name = %v, err = %v\n", src.name, err)
	}
	return n > 0
}

func flvToMp4(fromPath, toPath string) error {
	if err := exec.Command("ffmpeg", "-i", fromPath, "-codec", "copy", toPath).Run(); err != nil {
		return err
	}
	return os.Remove(fromPath)
}

func decideFilePrefix(member *memberData) string {
	if member.Prefix != nil {
		return *member.Prefix
	}
	return fmt.Sprintf("%v.", member.Name)
}

func decideFileFolder(member *memberData, config *defaultConfig) string {
	if member.Folder != nil {
		return *member.Folder
	}
	if config.DefaultFolder != nil {
		return *config.DefaultFolder
	}
	return defaultFolderPath
}
