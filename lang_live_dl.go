package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
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
	readConfigs()
	createFolders()

	configs := readConfigs()
	downloadTable := initDownloadTable(configs.Members)

	startDownloadThread(configs, downloadTable)
}

func readConfigs() configs {
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

func initDownloadTable(members []memberData) map[int]bool {
	downloadTable := map[int]bool{}
	for _, member := range members {
		downloadTable[member.Id] = false
	}
	return downloadTable
}

func buildStreamSources(config *defaultConfig, member *memberData) []streamSource {
	var sources []streamSource
	domains := []string{"video-ws-aws", "video-ws-hls-aws", "video-tx-int", "audio-tx-lh2"}
	postfixes := []string{"Y", "A"}
	exts := []string{"flv", "m3u8"}
	idx := 0
	for _, domain := range domains {
		for _, postfix := range postfixes {
			for _, ext := range exts {
				sources = append(sources, streamSource{
					name:         member.Name,
					url:          fmt.Sprintf("https://%v.lv-play.com/live/%v%v.%v", domain, member.Id, postfix, ext),
					filename:     fmt.Sprintf("%vi%v.", decideFilePrefix(member), idx),
					fileFolder:   decideFileFolder(member, config),
					checkOnly:    config.CheckOnly,
					enableNotify: member.EnableNotify,
				})
				idx += 1
			}
		}
	}
	return sources
}

func startDownloadThread(configs configs, downloadTable map[int]bool) {
	var downloadTableMtx sync.Mutex
	threadCount := 0
	threadLimit := len(configs.Members)
	for {
		for _, member := range configs.Members {
			func() {
				downloadTableMtx.Lock()
				defer downloadTableMtx.Unlock()
				if !downloadTable[member.Id] {
					downloadTable[member.Id] = true
					threadCount += 1
					if threadCount > threadLimit {
						fmt.Printf("current thread cnt = %v\n", threadCount)
					}
					go func(config defaultConfig, member memberData) {
						downloadForMember(&config, &member)
						downloadTableMtx.Lock()
						defer downloadTableMtx.Unlock()
						downloadTable[member.Id] = false
						threadCount -= 1
					}(configs.DefaultConfigs, member)
				}
			}()
		}
		time.Sleep(5300 * time.Millisecond)
	}
}

func downloadForMember(config *defaultConfig, member *memberData) {
	var wg sync.WaitGroup
	for _, src := range buildStreamSources(config, member) {
		wg.Add(1)
		go func(src streamSource) {
			defer wg.Done()
			downloadVideo(&src)
		}(src)
		time.Sleep(1100 * time.Millisecond)
	}
	wg.Wait()
}

func downloadVideo(src *streamSource) {
	filename := fmt.Sprintf("%v%v", src.filename, time.Now().Format("2006.01.02 15.04.05"))
	tempOutfilePath := filepath.Join(tempFolderPath, fmt.Sprintf("%v.mp4", filename))
	if err := exec.Command("ffmpeg", "-i", src.url, "-c", "copy", tempOutfilePath).Run(); err != nil {
		fmt.Printf("download video failed, name = %v, err = %v\n", src.name, err)
		return
	}
	if !src.checkOnly {
		outfilePath := filepath.Join(src.fileFolder, fmt.Sprintf("%v.mp4", filename))
		if err := toFinalMp4(tempOutfilePath, outfilePath); err != nil {
			fmt.Printf("to mp4 failed, err = %v\n", err)
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

func toFinalMp4(fromPath, toPath string) error {
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
