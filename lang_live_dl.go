package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sync/atomic"
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

type urlParam struct {
	domain  string
	postfix string
	ext     string
}

const (
	defaultFolderPath = "data"
	tempFolderPath    = "temp"

	defaultConfigJson = "default_config.json"
	configJson        = "config.json"
)

var (
	logFile           = setupLog()
	possibleUrlParams = setupPossibleUrlParams()
)

func main() {
	defer logFile.Close()

	readConfigs()
	createFolders()

	configs := readConfigs()
	sources := buildStreamSources(configs)
	downloadTable := initDownloadTable(sources)

	startDownloadThread(sources, downloadTable)
}

func setupLog() *os.File {
	logFile, err := os.Create("lang_live_dl.log")
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}

	log.SetOutput(logFile)
	return logFile
}

func setupPossibleUrlParams() []urlParam {
	var params []urlParam
	domains := []string{"video-ws-aws", "video-ws-hls-aws", "video-tx-int", "audio-tx-lh2"}
	postfixes := []string{"Y", "A"}
	exts := []string{"flv", "m3u8"}
	for _, domain := range domains {
		for _, postfix := range postfixes {
			for _, ext := range exts {
				params = append(params, urlParam{domain, postfix, ext})
			}
		}
	}
	log.Println("Possible Url Params:")
	for idx, param := range params {
		log.Printf("%v: %v\n", idx, param)
	}
	log.Println()
	return params
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

func initDownloadTable(sources []streamSource) []int32 {
	downloadTable := make([]int32, len(sources))
	for i := 0; i < len(sources); i++ {
		downloadTable[i] = 0
	}
	return downloadTable
}

func buildStreamSources(configs configs) []streamSource {
	var sources []streamSource
	for _, member := range configs.Members {
		idx := 0
		for _, param := range possibleUrlParams {
			sources = append(sources, streamSource{
				name:         member.Name,
				url:          fmt.Sprintf("https://%v.lv-play.com/live/%v%v.%v", param.domain, member.Id, param.postfix, param.ext),
				filename:     fmt.Sprintf("%vi%v.", decideFilePrefix(&member), idx),
				fileFolder:   decideFileFolder(&member, &configs.DefaultConfigs),
				checkOnly:    configs.DefaultConfigs.CheckOnly,
				enableNotify: member.EnableNotify,
			})
			idx += 1
		}
	}
	log.Println("All stream sources:")
	for i, src := range sources {
		log.Printf("%v: %v - %v - %v", i, src.name, src.filename, src.url)
	}
	log.Println("")
	return sources
}

func startDownloadThread(sources []streamSource, downloadTable []int32) {
	for {
		for idx, src := range sources {
			func() {
				if atomic.CompareAndSwapInt32(&downloadTable[idx], 0, 1) {
					go func(idx int, src streamSource) {
						if pingUrl(src.url) {
							downloadVideo(&src)
						}
						atomic.StoreInt32(&downloadTable[idx], 0)
					}(idx, src)
				}
				time.Sleep(time.Duration(20000/len(sources)) * time.Millisecond)
			}()
		}
	}
}

func pingUrl(url string) bool {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode/100 == 2
}

func downloadVideo(src *streamSource) {
	notify(src)
	filename := fmt.Sprintf("%v%v", src.filename, time.Now().Format("2006.01.02 15.04.05"))
	tempOutfilePath := filepath.Join(tempFolderPath, fmt.Sprintf("%v.mp4", filename))
	if err := exec.Command("ffmpeg", "-i", src.url, "-c", "copy", tempOutfilePath).Run(); err != nil {
		log.Printf("download video failed, name = %v, url = %v, err = %v\n", src.name, src.url, err)
		return
	}
	if !src.checkOnly {
		outfilePath := filepath.Join(src.fileFolder, fmt.Sprintf("%v.mp4", filename))
		if err := toFinalMp4(tempOutfilePath, outfilePath); err != nil {
			log.Printf("to mp4 failed, err = %v\n", err)
			return
		}
	} else {
		if err := os.Remove(tempOutfilePath); err != nil {
			log.Printf("remove file failed, err = %v", err)
			return
		}
	}
	log.Printf("download completed, name = %v\n", src.name)
}

func notify(src *streamSource) {
	if src.enableNotify {
		if err := beeep.Alert("stream start", src.name, ""); err != nil {
			log.Printf("Alert error = %v", err)
		}
	}
	log.Printf("stream start %v\n", src.name)
}

func toFinalMp4(fromPath, toPath string) error {
	dir := path.Dir(toPath)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		panic(fmt.Sprintf("create final folder error, path = %v, err = %v\n", dir, err))
	}
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
