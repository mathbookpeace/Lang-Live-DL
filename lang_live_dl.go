package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	downloadTable := initDownloadTable(configs.Members)
	var downloadTableMtx sync.Mutex
	threadCount := 0

	for {
		for _, member := range configs.Members {
			startDownloadThread(member, configs.DefaultConfigs, &downloadTableMtx, downloadTable, &threadCount)
		}
		if threadCount > len(configs.Members) {
			fmt.Printf("current thread cnt = %v\n", threadCount)
		}
		time.Sleep(2 * time.Second)
	}
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

func initDownloadTable(members []memberData) map[int]bool {
	downloadTable := map[int]bool{}
	for _, member := range members {
		downloadTable[member.Id] = false
	}
	return downloadTable
}

func startDownloadThread(member memberData, default_configs defaultConfig, downloadTableMtx *sync.Mutex, downloadTable map[int]bool, threadCount *int) {
	downloadTableMtx.Lock()
	defer downloadTableMtx.Unlock()
	if !downloadTable[member.Id] {
		downloadTable[member.Id] = true
		*threadCount += 1
		go func(member memberData) {
			downloadForMember(&member, &default_configs)
			downloadTableMtx.Lock()
			defer downloadTableMtx.Unlock()
			downloadTable[member.Id] = false
			*threadCount -= 1
		}(member)
	}
}

func downloadForMember(member *memberData, default_configs *defaultConfig) {
	filename := fmt.Sprintf("%v%v", decideFilePrefix(member), time.Now().Format("2006.01.02 15.04.05"))
	tempOutfilePath := filepath.Join(tempFolderPath, fmt.Sprintf("%v.flv", filename))
	if !downloadVideo(member, tempOutfilePath) {
		return
	}
	outfilePath := filepath.Join(decideFileFolder(member, default_configs), fmt.Sprintf("%v.mp4", filename))
	if !default_configs.CheckOnly {
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
	fmt.Printf("download completed, name = %v\n", member.Name)
}

func downloadVideo(member *memberData, outfilePath string) bool {
	url, err := getVideoUrl(member)
	if err != nil {
		fmt.Printf("get video url failed, err = %v", err)
		return false
	} else if url == "" {
		return false
	}
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("http get failed, name = %v, err = %v\n", member.Name, err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return false
	} else if resp.StatusCode != 200 {
		fmt.Printf("status = %v\n", resp.Status)
		return false
	}
	return writeRespToFile(member, outfilePath, resp)
}

func getVideoUrl(member *memberData) (string, error) {
	mainPage := fmt.Sprintf("https://www.lang.live/room/%v", member.Id)
	content, err := getWebContent(mainPage)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile("https://[^\"]*.lv-play.com") // compile the regex
	domain := re.FindString(content)
	if domain == "" {
		return "", nil
	}
	return fmt.Sprintf("%v/live/%vY.flv", domain, member.Id), nil
}

func getWebContent(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func writeRespToFile(member *memberData, outfilePath string, resp *http.Response) bool {
	outfile, err := os.Create(outfilePath)
	if err != nil {
		fmt.Printf("create file error, path = %v, err = %v\n", outfilePath, err)
		return false
	}
	defer outfile.Close()

	if member.EnableNotify {
		if err := beeep.Alert("stream start", member.Name, ""); err != nil {
			fmt.Printf("Alert error = %v", err)
		}
	}
	fmt.Printf("stream start %v\n", member.Name)

	n, err := io.Copy(outfile, resp.Body)
	if err != nil {
		fmt.Printf("download failed, name = %v, err = %v\n", member.Name, err)
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

func decideFileFolder(member *memberData, default_configs *defaultConfig) string {
	if member.Folder != nil {
		return *member.Folder
	}
	if default_configs.DefaultFolder != nil {
		return *default_configs.DefaultFolder
	}
	return defaultFolderPath
}
